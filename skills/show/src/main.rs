//! show — Operator skill (§9, AC114). Wraps `bug-render`.
use std::path::PathBuf;

use anyhow::Result;
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "show")]
struct Args {
    target: String,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };
    // Prefer the workspace-local bug-render binary; fall back to PATH.
    let candidates = [
        repo.join("target/release/bug-render"),
        repo.join("global/scripts/bug-render"),
        PathBuf::from("bug-render"),
    ];
    for cand in &candidates {
        if cand.is_file() || (!cand.is_absolute()) {
            let status = std::process::Command::new(cand)
                .arg(&args.target)
                .current_dir(&repo)
                .status();
            if let Ok(s) = status {
                if s.success() {
                    return Ok(());
                }
            }
        }
    }
    // Fallback: print bug JSON directly.
    if let Ok((path, _)) = operator_common::find_bug(&repo, &args.target) {
        println!("{}", std::fs::read_to_string(path)?);
        return Ok(());
    }
    anyhow::bail!(
        "show: bug-render not available and bug {} not found",
        args.target
    );
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
