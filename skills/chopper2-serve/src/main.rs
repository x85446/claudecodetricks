//! `chopper2-serve` (skill wrapper) — exec()s the real dashboard binary.
//!
//! The actual axum + askama dashboard is built from `agents/chopper2/dashboard/`
//! into `target/release/chopper2-serve-bin`. This skill is the trunk-callable
//! entry — it locates that binary relative to the repo root and `exec`s it,
//! so we keep one process tree and one log destination.

use std::path::PathBuf;
use std::process::Command;

use anyhow::{bail, Result};
use clap::Parser;

#[derive(Parser)]
#[command(name = "chopper2-serve")]
struct Args {
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long)]
    bind: Option<String>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let bin = args.repo_root.join("target/release/chopper2-serve-bin");
    if !bin.exists() {
        bail!(
            "dashboard binary missing at {} — did `cargo build --release --workspace` run?",
            bin.display()
        );
    }
    let mut cmd = Command::new(&bin);
    cmd.env("CHOPPER2_REPO_ROOT", &args.repo_root);
    if let Some(b) = &args.bind {
        cmd.env("CHOPPER2_BIND", b);
    }
    let status = cmd.status()?;
    std::process::exit(status.code().unwrap_or(1));
}
