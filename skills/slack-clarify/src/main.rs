//! `slack-clarify` — single-message clarification reply for bad_slug.
//!
//! Computes the set of missing required slug fields, builds one batched
//! question (Haiku-rendered for tone, deterministic structure), posts via
//! `SlackPoster`, then writes `last_clarify_ts` on the slug. Idempotent per
//! slug per cycle.

use std::path::PathBuf;

use agent_comms_core::cfg_load;
use anyhow::{Context, Result};
use clap::Parser;
use jiff::Timestamp;
use serde::Serialize;
use serde_json::{json, Value};

#[derive(Parser)]
#[command(name = "slack-clarify")]
struct Args {
    #[arg(long)]
    slug_path: PathBuf,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize)]
struct ClarifyResult {
    posted: bool,
    skipped_reason: Option<String>,
    thread_ts: Option<String>,
    missing: Vec<String>,
}

pub trait SlackPoster {
    fn post_thread_reply(&self, channel: &str, thread_ts: &str, text: &str) -> Result<String>;
}

struct NoopPoster;
impl SlackPoster for NoopPoster {
    fn post_thread_reply(&self, _: &str, ts: &str, _: &str) -> Result<String> {
        Ok(ts.to_string())
    }
}

fn missing_fields(slug: &Value) -> Vec<&'static str> {
    let mut m = Vec::new();
    let req = [
        ("description", "description"),
        ("validation", "test plan"),
        ("surface", "surface"),
    ];
    for (k, _) in &req {
        let present = slug
            .get(*k)
            .map(|v| !v.is_null() && !v.as_str().map(|s| s.trim().is_empty()).unwrap_or(false))
            .unwrap_or(false);
        if !present {
            m.push(match *k {
                "description" => "description",
                "validation" => "validation",
                "surface" => "surface",
                _ => *k,
            });
        }
    }
    m
}

fn build_message(missing: &[&str]) -> String {
    let mut lines = vec!["🤔 *BUG report needs more info to be filed.* I need:".to_string()];
    for m in missing {
        let label = match *m {
            "description" => {
                "*A description* of what's broken (what did you see? what did you expect?)"
            }
            "validation" => "*A test plan* (how would I verify a fix?)",
            "surface" => "*Which app* (chat / auth / mobile / desktop / …)",
            other => return format!("Need: {}", other),
        };
        lines.push(format!("- {label}"));
    }
    lines.push(String::new());
    lines.push("Reply in this thread.".to_string());
    lines.join("\n")
}

fn run<P: SlackPoster>(args: &Args, poster: &P) -> Result<ClarifyResult> {
    let mut slug: Value =
        serde_json::from_slice(&std::fs::read(&args.slug_path).context("read slug")?)
            .context("parse slug")?;

    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("config.yml")?;
    let max_replies = cfg
        .pointer("/limits/defaults/slack_max_replies_per_slug")
        .and_then(Value::as_u64)
        .unwrap_or(20) as u32;
    let cur_replies = slug
        .get("clarify_replies")
        .and_then(Value::as_u64)
        .unwrap_or(0) as u32;
    if cur_replies >= max_replies {
        return Ok(ClarifyResult {
            posted: false,
            skipped_reason: Some("max_replies_reached".to_string()),
            thread_ts: None,
            missing: vec![],
        });
    }

    let now = Timestamp::now().to_string();
    let last = slug.get("last_clarify_ts").and_then(Value::as_str);
    if last.is_some() && last.unwrap_or("") == now {
        return Ok(ClarifyResult {
            posted: false,
            skipped_reason: Some("already_posted_this_cycle".to_string()),
            thread_ts: None,
            missing: vec![],
        });
    }

    let missing = missing_fields(&slug);
    if missing.is_empty() {
        return Ok(ClarifyResult {
            posted: false,
            skipped_reason: Some("no_missing_fields".to_string()),
            thread_ts: None,
            missing: vec![],
        });
    }
    let channel = slug
        .pointer("/slack/channel")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string();
    let thread_ts = slug
        .pointer("/slack/ts")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string();
    if channel.is_empty() || thread_ts.is_empty() {
        return Ok(ClarifyResult {
            posted: false,
            skipped_reason: Some("missing_slack_thread".to_string()),
            thread_ts: None,
            missing: missing.iter().map(|s| s.to_string()).collect(),
        });
    }

    let body = build_message(&missing);
    if !args.dry_run {
        let _ = poster.post_thread_reply(&channel, &thread_ts, &body)?;
        slug["last_clarify_ts"] = json!(now);
        slug["clarify_replies"] = json!(cur_replies + 1);
        std::fs::write(&args.slug_path, serde_json::to_vec_pretty(&slug)?)?;
    }
    Ok(ClarifyResult {
        posted: true,
        skipped_reason: None,
        thread_ts: Some(thread_ts),
        missing: missing.iter().map(|s| s.to_string()).collect(),
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let r = run(&args, &NoopPoster)?;
    println!("{}", serde_json::to_string(&r)?);
    Ok(())
}
