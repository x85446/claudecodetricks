use std::fs;
use std::path::PathBuf;

use anyhow::{bail, Context, Result};
use clap::Parser;
use serde_json::Value;

#[derive(Parser, Debug)]
#[command(
    name = "read-bug-history",
    about = "Print a bug's events[] array (newest first)."
)]
struct Args {
    #[arg()]
    bug_id: String,
    #[arg(long)]
    event_type: Option<String>,
    #[arg(long, default_value_t = 10)]
    limit: usize,
    #[arg(long, default_value = "from-chopper")]
    from_dir: String,
    #[arg(long)]
    cwd: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
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
    let bug: Value = serde_json::from_str(&fs::read_to_string(&bug_path)?)
        .with_context(|| format!("parse {}", bug_path.display()))?;
    let mut events: Vec<Value> = bug
        .get("events")
        .and_then(|v| v.as_array())
        .cloned()
        .unwrap_or_default();
    events.reverse(); // newest first
    if let Some(t) = &args.event_type {
        events.retain(|e| e.get("type").and_then(|x| x.as_str()) == Some(t.as_str()));
    }
    if args.limit > 0 {
        events.truncate(args.limit);
    }
    println!("{}", serde_json::to_string_pretty(&events)?);
    Ok(())
}
