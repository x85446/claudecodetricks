//! list — Operator skill (§9). Read-only filter on bugs/.
use std::path::PathBuf;

use agent_comms_core::schema::Bug;
use anyhow::Result;
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "list")]
struct Args {
    #[arg(long)]
    state: Option<String>,
    #[arg(long)]
    repo_filter: Option<String>,
    #[arg(long)]
    assignee: Option<String>,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };
    let now = jiff::Timestamp::now();
    let mut count = 0;
    for path in operator_common::iter_bug_files(&repo) {
        let raw = std::fs::read_to_string(&path)?;
        let Ok(bug): std::result::Result<Bug, _> = serde_json::from_str(&raw) else {
            continue;
        };
        if let Some(s) = &args.state {
            if bug.ghlstate.as_str() != s {
                continue;
            }
        }
        if let Some(r) = &args.repo_filter {
            if bug.repo.as_deref() != Some(r.as_str()) {
                continue;
            }
        }
        if let Some(a) = &args.assignee {
            if bug.assignee.as_deref() != Some(a.as_str()) {
                continue;
            }
        }
        let since_secs = bug
            .current_state
            .since
            .parse::<jiff::Timestamp>()
            .map(|t| t.as_second())
            .unwrap_or(now.as_second());
        let age_s = (now.as_second() - since_secs).max(0);
        let age = format_age(age_s);
        println!(
            "{}\t{}\t{}\t{}\t{}",
            bug.id,
            bug.ghlstate.as_str(),
            bug.assignee.as_deref().unwrap_or("-"),
            age,
            bug.title
        );
        count += 1;
    }
    eprintln!("{count} bug(s) matched.");
    Ok(())
}

fn format_age(secs: i64) -> String {
    if secs < 60 {
        format!("{}s", secs)
    } else if secs < 3600 {
        format!("{}m", secs / 60)
    } else if secs < 86400 {
        format!("{}h", secs / 3600)
    } else {
        format!("{}d", secs / 86400)
    }
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
