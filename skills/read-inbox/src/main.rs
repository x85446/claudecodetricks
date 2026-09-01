use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

use anyhow::{Context, Result};
use clap::Parser;
use serde_json::{json, Value};

#[derive(Parser, Debug)]
#[command(name = "read-inbox", about = "List the leaf's from-chopper/ contents.")]
struct Args {
    #[arg(long)]
    cwd: Option<PathBuf>,
    #[arg(long, default_value_t = false)]
    include_blocked: bool,
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

fn age_seconds(p: &Path) -> u64 {
    fs::metadata(p)
        .and_then(|m| m.modified())
        .map(|t| {
            SystemTime::now()
                .duration_since(t)
                .map(|d| d.as_secs())
                .unwrap_or(0)
        })
        .unwrap_or(0)
}

fn blocker_resolved(repo_root: &Path, blocker_id: &str) -> bool {
    let candidates = [
        repo_root.join("bugs").join(format!("{blocker_id}.json")),
        repo_root
            .join("bugs/closed")
            .join(format!("{blocker_id}.json")),
        repo_root
            .join("bugs/_blocked")
            .join(format!("{blocker_id}.json")),
    ];
    for c in &candidates {
        if let Ok(text) = fs::read_to_string(c) {
            if let Ok(v) = serde_json::from_str::<Value>(&text) {
                if let Some(state) = v.get("ghlstate").and_then(|s| s.as_str()) {
                    return matches!(state, "verified" | "closed");
                }
            }
        }
    }
    false
}

fn main() -> Result<()> {
    let args = Args::parse();
    let cwd = args
        .cwd
        .clone()
        .unwrap_or_else(|| std::env::current_dir().expect("cwd"));
    let inbox = cwd.join("from-chopper");
    let repo_root = find_git_root(&cwd).unwrap_or(cwd.clone());

    let mut out: Vec<Value> = vec![];

    if !inbox.exists() {
        println!("[]");
        return Ok(());
    }

    let mut entries: Vec<PathBuf> = fs::read_dir(&inbox)
        .with_context(|| format!("read_dir {}", inbox.display()))?
        .filter_map(|r| r.ok())
        .map(|e| e.path())
        .filter(|p| p.extension().map(|x| x == "json").unwrap_or(false))
        .collect();
    entries.sort();

    for p in entries {
        let text = match fs::read_to_string(&p) {
            Ok(t) => t,
            Err(_) => continue,
        };
        let v: Value = match serde_json::from_str(&text) {
            Ok(v) => v,
            Err(_) => continue,
        };
        let id_or_slug = v
            .get("id")
            .and_then(|x| x.as_str())
            .or_else(|| v.get("slug").and_then(|x| x.as_str()))
            .unwrap_or("")
            .to_string();
        let ghlstate = v
            .get("ghlstate")
            .and_then(|x| x.as_str())
            .unwrap_or("")
            .to_string();
        let priority = v
            .get("priority")
            .and_then(|x| x.as_str())
            .unwrap_or("")
            .to_string();
        let blockers: Vec<String> = v
            .get("blocked_by")
            .and_then(|x| x.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|s| s.as_str().map(|x| x.to_string()))
                    .collect()
            })
            .unwrap_or_default();
        let blockers_open: Vec<String> = blockers
            .iter()
            .filter(|b| !blocker_resolved(&repo_root, b))
            .cloned()
            .collect();
        let blocked = !blockers_open.is_empty();
        if blocked && !args.include_blocked {
            continue;
        }
        out.push(json!({
            "path": p.display().to_string(),
            "bug_id_or_slug": id_or_slug,
            "ghlstate": ghlstate,
            "priority": priority,
            "age_seconds": age_seconds(&p),
            "blocked": blocked,
            "blockers_open": blockers_open,
        }));
    }

    println!("{}", serde_json::to_string_pretty(&out)?);
    Ok(())
}
