use std::fs;
use std::path::PathBuf;

use anyhow::{bail, Context, Result};
use clap::Parser;
use serde_json::{json, Value};

#[derive(Parser, Debug)]
#[command(
    name = "claim-bug",
    about = "Mark a bug as in_progress; do not move it."
)]
struct Args {
    #[arg()]
    bug_id: String,
    #[arg(long, default_value = "from-chopper")]
    from_dir: String,
    #[arg(long)]
    actor: Option<String>,
    #[arg(long, default_value_t = false)]
    dry_run: bool,
    #[arg(long)]
    cwd: Option<PathBuf>,
}

fn dry_run_enabled(arg: bool) -> bool {
    arg || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false)
}

fn agent_name() -> String {
    std::env::var("AGENT_NAME").unwrap_or_else(|_| "unknown-leaf".to_string())
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry_run = dry_run_enabled(args.dry_run);
    let cwd = args
        .cwd
        .clone()
        .unwrap_or_else(|| std::env::current_dir().expect("cwd"));
    let bug_path = cwd
        .join(&args.from_dir)
        .join(format!("{}.json", args.bug_id));
    if !bug_path.exists() {
        bail!("bug not found: {}", bug_path.display());
    }
    if dry_run {
        println!("DRY_RUN: would set in_progress on {}", args.bug_id);
        return Ok(());
    }
    let mut bug: Value = serde_json::from_str(&fs::read_to_string(&bug_path)?)
        .with_context(|| format!("parse {}", bug_path.display()))?;
    let now = jiff::Timestamp::now().to_string();

    bug["ghlstate"] = json!("in_progress");
    bug["current_state"] = json!({"since": now, "note": "claimed"});

    let mut event = json!({
        "ts": now,
        "type": "claimed",
        "by": agent_name(),
    });
    if let Some(actor) = &args.actor {
        event["actor"] = json!(actor);
    }
    bug.as_object_mut()
        .unwrap()
        .entry("events")
        .or_insert_with(|| Value::Array(vec![]))
        .as_array_mut()
        .unwrap()
        .push(event);

    let tmp = bug_path.with_extension("json.tmp");
    fs::write(&tmp, serde_json::to_vec_pretty(&bug)?)?;
    fs::rename(&tmp, &bug_path)?;

    let related = bug
        .get("related_bugs")
        .cloned()
        .unwrap_or(Value::Array(vec![]));
    println!(
        "{}",
        serde_json::to_string_pretty(&json!({
            "bug_id": args.bug_id,
            "ghlstate": "in_progress",
            "related_bugs": related,
        }))?
    );
    Ok(())
}
