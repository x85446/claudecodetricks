use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use clap::Parser;
use serde_json::{json, Value};

#[derive(Parser, Debug)]
#[command(
    name = "update-bug",
    about = "Edit a bug record in place; optionally bounce to needs_info."
)]
struct Args {
    #[arg()]
    bug_id: String,
    #[arg(long)]
    ghlstate: Option<String>,
    #[arg(long)]
    note: Option<String>,
    #[arg(long)]
    reason: Option<String>,
    #[arg(long, default_value_t = false)]
    infra_failure: bool,
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

fn load_bug(path: &Path) -> Result<Value> {
    let text = fs::read_to_string(path).with_context(|| format!("read {}", path.display()))?;
    let v: Value =
        serde_json::from_str(&text).with_context(|| format!("parse {}", path.display()))?;
    Ok(v)
}

fn save_bug_atomic(path: &Path, v: &Value) -> Result<()> {
    let parent = path.parent().context("path missing parent")?;
    let tmp = parent.join(format!(
        ".{}.tmp",
        path.file_name().unwrap_or_default().to_string_lossy()
    ));
    fs::write(&tmp, serde_json::to_vec_pretty(v)?)?;
    fs::rename(tmp, path)?;
    Ok(())
}

fn append_event(bug: &mut Value, event: Value) {
    let arr = bug
        .as_object_mut()
        .expect("bug must be object")
        .entry("events")
        .or_insert_with(|| Value::Array(Vec::new()));
    arr.as_array_mut()
        .expect("events must be array")
        .push(event);
}

fn run_git_mv(repo: &Path, from: &Path, to: &Path, dry_run: bool) -> Result<()> {
    if dry_run {
        eprintln!("DRY_RUN: git mv {} {}", from.display(), to.display());
        return Ok(());
    }
    let from_rel = from.strip_prefix(repo).unwrap_or(from);
    let to_rel = to.strip_prefix(repo).unwrap_or(to);
    let status = Command::new("git")
        .arg("mv")
        .arg(from_rel)
        .arg(to_rel)
        .current_dir(repo)
        .status()
        .with_context(|| format!("spawn git mv in {}", repo.display()))?;
    if !status.success() {
        bail!(
            "git mv failed: {} -> {} (exit {:?})",
            from.display(),
            to.display(),
            status.code()
        );
    }
    Ok(())
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry_run = dry_run_enabled(args.dry_run);
    let cwd = match args.cwd {
        Some(p) => p,
        None => std::env::current_dir().context("get cwd")?,
    };

    if args.infra_failure && args.ghlstate.is_some() {
        bail!("--infra-failure cannot be combined with --ghlstate");
    }
    if let Some(s) = &args.ghlstate {
        if s != "needs_info" {
            bail!(
                "update-bug only allows --ghlstate=needs_info from leaves; got {}",
                s
            );
        }
    }

    let from_dir = cwd.join(&args.from_dir);
    let bug_path = from_dir.join(format!("{}.json", args.bug_id));
    if !bug_path.exists() {
        bail!("bug not found: {}", bug_path.display());
    }

    let mut bug = load_bug(&bug_path)?;
    let now = jiff::Timestamp::now().to_string();
    let by = agent_name();

    let mut event = json!({
        "ts": now,
        "by": by,
    });
    if let Some(actor) = &args.actor {
        event["actor"] = json!(actor);
    }

    if args.infra_failure {
        event["type"] = json!("infra_failure");
        if let Some(note) = &args.note {
            event["note"] = json!(note);
        }
        bug["infra_failure"] = json!(true);
    } else if args.ghlstate.as_deref() == Some("needs_info") {
        event["type"] = json!("needs_info");
        bug["ghlstate"] = json!("needs_info");
        if let Some(reason) = &args.reason {
            bug["ghlstatereason"] = json!(reason);
            event["reason"] = json!(reason);
        }
        if let Some(note) = &args.note {
            event["note"] = json!(note);
        }
        bug["current_state"] = json!({"since": now, "note": args.note.clone()});
    } else {
        event["type"] = json!("note");
        if let Some(note) = &args.note {
            event["note"] = json!(note);
        }
    }

    append_event(&mut bug, event);

    if dry_run {
        println!("DRY_RUN: would update {}", bug_path.display());
        if args.ghlstate.as_deref() == Some("needs_info") {
            println!("DRY_RUN: would git mv to to-chopper/");
        }
        return Ok(());
    }

    save_bug_atomic(&bug_path, &bug)?;

    if args.ghlstate.as_deref() == Some("needs_info") {
        let to_path = cwd.join("to-chopper").join(format!("{}.json", args.bug_id));
        // The git repo containing this file is the agent-comms repo (cwd's ancestor with a .git dir).
        let repo = find_git_root(&cwd).unwrap_or_else(|| cwd.clone());
        run_git_mv(&repo, &bug_path, &to_path, false)?;
    }

    println!("updated {}", args.bug_id);
    Ok(())
}

fn find_git_root(start: &Path) -> Option<PathBuf> {
    let mut cur = start.to_path_buf();
    loop {
        if cur.join(".git").exists() {
            return Some(cur);
        }
        if !cur.pop() {
            return None;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn append_event_pushes() {
        let mut v = json!({"events": []});
        append_event(&mut v, json!({"type": "x"}));
        assert_eq!(v["events"].as_array().unwrap().len(), 1);
    }

    #[test]
    fn append_event_creates_array_if_missing() {
        let mut v = json!({});
        append_event(&mut v, json!({"type": "x"}));
        assert_eq!(v["events"].as_array().unwrap().len(), 1);
    }
}
