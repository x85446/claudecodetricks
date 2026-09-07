//! `notify` — typed Slack notification with cooldown + failure persistence.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use agent_comms_core::cfg_load;
use anyhow::{Context, Result};
use clap::Parser;
use jiff::Timestamp;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

#[derive(Parser)]
#[command(name = "notify")]
struct Args {
    #[arg(long = "type")]
    notif_type: String,
    #[arg(long, default_value = "{}")]
    context: String,
    #[arg(long)]
    repo: Option<String>,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize)]
struct NotifyResult {
    posted: bool,
    suppressed: bool,
    recipients: Vec<String>,
    cooldown_key: String,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
struct Cooldown {
    last_fired_at: String,
    suppressed_since_last: u32,
}

pub trait SlackPoster {
    fn post(&self, channel: &str, text: &str) -> Result<()>;
}

struct NoopPoster;
impl SlackPoster for NoopPoster {
    fn post(&self, _: &str, _: &str) -> Result<()> {
        Ok(())
    }
}

fn cooldown_key(notif_type: &str, ctx: &Value) -> String {
    let key = ctx
        .get("bug_id")
        .or_else(|| ctx.get("host"))
        .or_else(|| ctx.get("slug_id"))
        .and_then(Value::as_str)
        .unwrap_or("_global");
    format!("{notif_type}:{key}")
}

fn read_cooldowns(repo_root: &Path) -> BTreeMap<String, Cooldown> {
    let path = repo_root.join("agents/chopper2/reports/notify-cooldowns.json");
    let bytes = match std::fs::read(&path) {
        Ok(b) => b,
        Err(_) => return BTreeMap::new(),
    };
    serde_json::from_slice(&bytes).unwrap_or_default()
}

fn write_cooldowns(repo_root: &Path, map: &BTreeMap<String, Cooldown>) -> Result<()> {
    let path = repo_root.join("agents/chopper2/reports/notify-cooldowns.json");
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).ok();
    }
    let pretty = serde_json::to_vec_pretty(map)?;
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, pretty)?;
    std::fs::rename(&tmp, &path)?;
    Ok(())
}

fn render_template(notif_type: &str, ctx: &Value) -> String {
    let bug_id = ctx.get("bug_id").and_then(Value::as_str).unwrap_or("BUG-?");
    match notif_type {
        "new_bug_in_who_codes" => format!(
            "🧑 {bug_id} needs your routing decision in {}.",
            ctx.get("repo").and_then(Value::as_str).unwrap_or("?")
        ),
        "p0_bug_filed" => format!(
            "🚨 P0 {bug_id} filed: {} (repo: {}).",
            ctx.get("title").and_then(Value::as_str).unwrap_or("?"),
            ctx.get("repo").and_then(Value::as_str).unwrap_or("?")
        ),
        "bug_verified" => format!("✅ {bug_id} verified. Closing."),
        "bug_closed" => format!("✅ {bug_id} closed."),
        "slug_dead_letter" => format!(
            "⚠ Slug {} dead-lettered.",
            ctx.get("slug_id").and_then(Value::as_str).unwrap_or("?")
        ),
        "auto_recovery_triggered" => format!(
            "🔧 {bug_id} auto-restored from {}. Breaking commit: {}.",
            ctx.get("restored_sha")
                .and_then(Value::as_str)
                .unwrap_or("?"),
            ctx.get("broke_at_sha")
                .and_then(Value::as_str)
                .unwrap_or("?")
        ),
        "manifest_drift" => format!(
            "📡 Host {} silent.",
            ctx.get("host").and_then(Value::as_str).unwrap_or("?")
        ),
        "escalation_needed" => format!("🔁 {bug_id} hit fix-attempt cap. Operator review needed."),
        "skill_error" => format!(
            "🐛 Skill `{}` errored on {bug_id}.",
            ctx.get("skill").and_then(Value::as_str).unwrap_or("?")
        ),
        "skill_stuck" => format!(
            "🛑 {bug_id} stuck — skill `{}` exhausted retries.",
            ctx.get("skill").and_then(Value::as_str).unwrap_or("?")
        ),
        "crash_recovered" => format!(
            "💥 Host {} crash-recovered.",
            ctx.get("host").and_then(Value::as_str).unwrap_or("?")
        ),
        "external_bug_filed" => format!("🌐 External {bug_id} filed."),
        "wrong_repo_reroute" => format!(
            "↪ {bug_id} rerouted to {}.",
            ctx.get("new_repo").and_then(Value::as_str).unwrap_or("?")
        ),
        "infra_failure_persistent" => format!("🛠 {bug_id}: persistent infra failure."),
        "merge_conflict" => format!("⚔ {bug_id} merge conflict."),
        "non_allowlist_mention" => "👀 Mentioned in non-allowlisted Slack channel.".to_string(),
        "slack_token_age_critical" => {
            "🔑 Slack token >85d old. Rotate before 90d expiry.".to_string()
        }
        "bug_stale_who_codes" => format!("⏰ {bug_id} awaiting human triage."),
        "bug_stale_in_progress" => format!("⏰ {bug_id} stuck in_progress."),
        other => format!("[{other}] {}", ctx),
    }
}

fn write_failure(repo_root: &Path, attempt_payload: &Value) -> Result<()> {
    let date = Timestamp::now().to_string()[..10].to_string();
    let path = repo_root
        .join("agents/chopper2/reports")
        .join(format!("notify-failures-{date}.json"));
    if let Some(p) = path.parent() {
        std::fs::create_dir_all(p).ok();
    }
    let mut existing: Vec<Value> = if path.exists() {
        serde_json::from_slice(&std::fs::read(&path)?).unwrap_or_default()
    } else {
        Vec::new()
    };
    existing.push(attempt_payload.clone());
    std::fs::write(&path, serde_json::to_vec_pretty(&existing)?)?;
    Ok(())
}

fn append_audit(repo_root: &Path, entry: &Value) -> Result<()> {
    let date = Timestamp::now().to_string()[..10].to_string();
    let path = repo_root
        .join("agents/chopper2/reports")
        .join(format!("{date}.json"));
    if let Some(p) = path.parent() {
        std::fs::create_dir_all(p).ok();
    }
    let mut existing: Vec<Value> = if path.exists() {
        serde_json::from_slice(&std::fs::read(&path)?).unwrap_or_default()
    } else {
        Vec::new()
    };
    existing.push(entry.clone());
    std::fs::write(&path, serde_json::to_vec_pretty(&existing)?)?;
    Ok(())
}

fn run<P: SlackPoster>(args: &Args, poster: &P) -> Result<NotifyResult> {
    let context: Value = serde_json::from_str(&args.context).context("parse context json")?;
    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("load config.yml")?;
    let now = Timestamp::now();
    let key = cooldown_key(&args.notif_type, &context);
    let mut cool = read_cooldowns(&args.repo_root);

    // notifications.per_type.<notif_type>.rate_limit.window_minutes,
    // falling back to notifications.defaults.rate_limit.window_minutes.
    let window_minutes = cfg
        .pointer(&format!(
            "/notifications/per_type/{}/rate_limit/window_minutes",
            args.notif_type
        ))
        .and_then(Value::as_u64)
        .or_else(|| {
            cfg.pointer("/notifications/defaults/rate_limit/window_minutes")
                .and_then(Value::as_u64)
        })
        .unwrap_or(60);
    let window_secs = (window_minutes as i64) * 60;

    let suppress = match cool.get(&key) {
        Some(c) => match c.last_fired_at.parse::<Timestamp>() {
            Ok(t) => (now.as_second() - t.as_second()) < window_secs,
            Err(_) => false,
        },
        None => false,
    };

    // Resolve subscribers from repos.per_repo.<repo>.notifications.subscriptions.<notif_type>
    // → list of alias names → repos.per_repo.<repo>.notifications.aliases.<alias>.slack_id.
    let repo_key = args.repo.as_deref().unwrap_or("_global");
    let mut recipients: Vec<String> = Vec::new();
    if let Some(repos_root) = cfg.pointer(&format!("/repos/per_repo/{repo_key}/notifications")) {
        if let Some(subs) = repos_root
            .pointer(&format!("/subscriptions/{}", args.notif_type))
            .and_then(Value::as_array)
        {
            for alias in subs.iter().filter_map(Value::as_str) {
                if let Some(slack_id) = repos_root
                    .pointer(&format!("/aliases/{alias}/slack_id"))
                    .and_then(Value::as_str)
                {
                    recipients.push(slack_id.to_string());
                }
            }
        }
    }
    if recipients.is_empty() {
        recipients.push("#chopper2-reports".to_string());
    }

    let body = render_template(&args.notif_type, &context);

    if suppress {
        let bumped = {
            let entry = cool.entry(key.clone()).or_insert(Cooldown {
                last_fired_at: now.to_string(),
                suppressed_since_last: 0,
            });
            entry.suppressed_since_last += 1;
            entry.suppressed_since_last
        };
        if !args.dry_run {
            write_cooldowns(&args.repo_root, &cool)?;
            append_audit(
                &args.repo_root,
                &json!({
                    "ts": now.to_string(),
                    "notif_type": args.notif_type,
                    "key": key,
                    "recipients": recipients,
                    "suppressed": true,
                    "suppressed_count": bumped,
                }),
            )?;
        }
        return Ok(NotifyResult {
            posted: false,
            suppressed: true,
            recipients,
            cooldown_key: key,
        });
    }

    // Post (or pretend to in dry-run); fall back to failures file on error.
    let mut posted = true;
    if !args.dry_run {
        for ch in &recipients {
            if let Err(e) = poster.post(ch, &body) {
                posted = false;
                let attempt = json!({
                    "ts":         now.to_string(),
                    "notif_type": args.notif_type,
                    "attempt":    1,
                    "error":      e.to_string(),
                    "payload":    { "channel": ch, "text": body },
                    "delivered_at": Value::Null,
                });
                let _ = write_failure(&args.repo_root, &attempt);
            }
        }
    }

    cool.insert(
        key.clone(),
        Cooldown {
            last_fired_at: now.to_string(),
            suppressed_since_last: 0,
        },
    );
    if !args.dry_run {
        // Prune expired entries in the same write.
        cool.retain(|_, c| {
            c.last_fired_at
                .parse::<Timestamp>()
                .map(|t| (now.as_second() - t.as_second()) < window_secs * 24)
                .unwrap_or(true)
        });
        write_cooldowns(&args.repo_root, &cool)?;
        append_audit(
            &args.repo_root,
            &json!({
                "ts": now.to_string(),
                "notif_type": args.notif_type,
                "key": key,
                "recipients": recipients,
                "suppressed": false,
                "suppressed_count": 0,
            }),
        )?;
    }
    Ok(NotifyResult {
        posted,
        suppressed: false,
        recipients,
        cooldown_key: key,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let r = run(&args, &NoopPoster)?;
    println!("{}", serde_json::to_string(&r)?);
    Ok(())
}
