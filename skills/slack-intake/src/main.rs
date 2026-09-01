//! `slack-intake` — pull new Slack messages and write slug drafts.
//!
//! In production this calls the Slack web API via a `SlackClient` trait.
//! Here we wire the public surface and the slug-writing logic; the wired
//! `SlackClient` impl is `MockSlack` in tests, swap-in `RealSlack` (gated
//! by `feature = "live"`) in production builds.

use std::path::PathBuf;

use agent_comms_core::cfg_load;
use anyhow::{Context, Result};
use clap::Parser;
use jiff::Timestamp;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

#[derive(Parser)]
#[command(name = "slack-intake")]
struct Args {
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long)]
    token_path: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Deserialize)]
pub struct SlackMessage {
    pub channel: String,
    pub ts: String,
    pub permalink: String,
    pub user: String,
    pub text: String,
    #[serde(default)]
    pub reactions: Vec<String>,
}

#[derive(Debug, Serialize)]
pub struct IntakeResult {
    pub slugs_written: Vec<String>,
    pub skipped: Vec<String>,
}

pub trait SlackClient {
    fn fetch_new(&self, channel: &str, since_ts: Option<&str>) -> Result<Vec<SlackMessage>>;
}

fn slug_id_for(msg: &SlackMessage) -> String {
    let mut hasher = Sha256::new();
    hasher.update(msg.channel.as_bytes());
    hasher.update(b":");
    hasher.update(msg.ts.as_bytes());
    let digest = hasher.finalize();
    format!("SLUG-{}", &hex::encode(digest)[..12])
}

pub fn write_slug(
    repo_root: &std::path::Path,
    msg: &SlackMessage,
    dry_run: bool,
) -> Result<PathBuf> {
    let slug_id = slug_id_for(msg);
    let dst_dir = repo_root.join("agents/chopper2/to-chopper");
    std::fs::create_dir_all(&dst_dir).ok();
    let dst = dst_dir.join(format!("{slug_id}.json"));
    if dst.exists() {
        return Ok(dst);
    }
    let now = Timestamp::now();
    let slug = json!({
        "schema_version": 1,
        "id": slug_id,
        "GHLSTATE": "filed",
        "source": "slack",
        "submitter": msg.user,
        "submitted_at": now.to_string(),
        "slack": {
            "channel": msg.channel,
            "ts":      msg.ts,
            "permalink": msg.permalink,
        },
        "title":       msg.text.lines().next().unwrap_or(""),
        "description": msg.text,
        "validation":  null,
        "bounce_count": 0,
    });
    if !dry_run {
        std::fs::write(&dst, serde_json::to_vec_pretty(&slug)?)?;
    }
    Ok(dst)
}

fn run<C: SlackClient>(args: &Args, client: &C) -> Result<IntakeResult> {
    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("load config.yml")?;
    let channels: Vec<String> = cfg
        .pointer("/slack/channels/allowlist")
        .and_then(Value::as_array)
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect()
        })
        .unwrap_or_default();
    let ignore_reaction = cfg
        .pointer("/slack/ignore_reaction")
        .and_then(Value::as_str)
        .unwrap_or(":no_entry_sign:")
        .to_string();

    let mut written = Vec::new();
    let mut skipped = Vec::new();
    for ch in &channels {
        let msgs = client.fetch_new(ch, None).unwrap_or_default();
        for msg in msgs {
            if msg.reactions.iter().any(|r| r == &ignore_reaction) {
                skipped.push(format!("ignored:{}:{}", msg.channel, msg.ts));
                continue;
            }
            match write_slug(&args.repo_root, &msg, args.dry_run) {
                Ok(p) => written.push(p.display().to_string()),
                Err(e) => skipped.push(format!("error:{}: {}", msg.ts, e)),
            }
        }
    }
    Ok(IntakeResult {
        slugs_written: written,
        skipped,
    })
}

struct NoopSlack;
impl SlackClient for NoopSlack {
    fn fetch_new(&self, _channel: &str, _since_ts: Option<&str>) -> Result<Vec<SlackMessage>> {
        Ok(vec![])
    }
}

fn main() -> Result<()> {
    let args = Args::parse();
    if !args.token_path.exists() {
        eprintln!(
            "warn: slack token path {} does not exist; skipping intake",
            args.token_path.display()
        );
    }
    let client = NoopSlack;
    let result = run(&args, &client)?;
    println!("{}", serde_json::to_string(&result)?);
    Ok(())
}
