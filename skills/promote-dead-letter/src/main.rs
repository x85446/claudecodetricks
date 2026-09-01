//! promote-dead-letter — Operator skill (§9, IT-S9, AC9).
use std::path::PathBuf;

use agent_comms_core::{
    ids::next_bug_id,
    paths::{from_chopper_dir, index_state_path, slugs_dead_letter_dir},
};
use agent_comms_git::{GitRunner, RealGit};
use anyhow::{anyhow, Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "promote-dead-letter")]
struct Args {
    slug: String,
    #[arg(long)]
    dry_run: bool,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry = args.dry_run || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false);
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };
    let dl_dir = slugs_dead_letter_dir(&repo);
    let path = find_slug(&dl_dir, &args.slug)?;

    let slug_v: serde_json::Value = serde_json::from_str(&std::fs::read_to_string(&path)?)?;
    let target_repo = slug_v
        .get("repo")
        .and_then(|x| x.as_str())
        .unwrap_or("df-chat");
    let role = "coder";

    if let Some(_existing) = scan_for_existing(&repo, &args.slug)? {
        eprintln!("promote-dead-letter: bug already exists for slug — no-op");
        return Ok(());
    }

    if dry {
        let target_dir = from_chopper_dir(&repo, target_repo, role);
        println!(
            "would: allocate BUG ID ; write bugs/<id>.json with accepted_by=operator ; \
             rm {} ; route to {}",
            path.display(),
            target_dir.display()
        );
        return Ok(());
    }

    let bug_id = next_bug_id(&index_state_path(&repo)).with_context(|| "next_bug_id")?;

    let now = jiff::Timestamp::now();
    let bug_json = serde_json::json!({
        "schema_version": 1,
        "ghlstate": "assigned",
        "id": bug_id,
        "slug": args.slug,
        "title": slug_v.get("title").and_then(|s| s.as_str()).unwrap_or(""),
        "priority": slug_v.get("priority").and_then(|s| s.as_str()).unwrap_or("P3"),
        "category": slug_v.get("category").and_then(|s| s.as_str()).unwrap_or("misc"),
        "repo": target_repo,
        "filed_by": slug_v.get("filed_by").and_then(|s| s.as_str()).unwrap_or("operator"),
        "filed_at": now.to_string(),
        "assignee": format!("{target_repo}/{role}"),
        "fix_attempts": 0,
        "skill_retries": {},
        "infra_failure": false,
        "current_state": { "since": now.to_string() },
        "description": slug_v.get("description").and_then(|s| s.as_str()).unwrap_or(""),
        "validation": slug_v.get("validation").and_then(|s| s.as_str()).unwrap_or(""),
        "events": [
            {
                "ts": now.to_string(),
                "type": "promoted_from_dead_letter",
                "by": "operator",
                "actor": "operator",
                "accepted_by": "operator",
                "from_slug": args.slug,
            }
        ],
        "children": [], "blocked_by": [], "blocks": [], "related_bugs": [],
    });

    let dest_dir = from_chopper_dir(&repo, target_repo, role);
    std::fs::create_dir_all(&dest_dir)?;
    let bug_path = dest_dir.join(format!("{bug_id}.json"));
    std::fs::write(&bug_path, serde_json::to_string_pretty(&bug_json)?)?;
    let runner = RealGit;
    runner.run(
        &[
            "add",
            bug_path.strip_prefix(&repo).unwrap().to_str().unwrap(),
        ],
        &repo,
    )?;
    let rel = path.strip_prefix(&repo).unwrap_or(&path);
    runner.run(&["rm", "-f", rel.to_str().unwrap()], &repo)?;
    operator_common::commit_and_push(
        &runner,
        &repo,
        &format!(
            "operator: promote dead-letter slug {} → {}",
            args.slug, bug_id
        ),
    )?;
    println!("{bug_id}");
    Ok(())
}

fn find_slug(dir: &std::path::Path, slug: &str) -> Result<std::path::PathBuf> {
    if !dir.exists() {
        return Err(anyhow!("dead-letter dir missing"));
    }
    for entry in walkdir::WalkDir::new(dir).into_iter().flatten() {
        if !entry.file_type().is_file() {
            continue;
        }
        let p = entry.path();
        if let Ok(raw) = std::fs::read_to_string(p) {
            if let Ok(v) = serde_json::from_str::<serde_json::Value>(&raw) {
                if v.get("slug").and_then(|s| s.as_str()) == Some(slug) {
                    return Ok(p.to_path_buf());
                }
            }
        }
        if p.file_stem().and_then(|s| s.to_str()) == Some(slug) {
            return Ok(p.to_path_buf());
        }
    }
    Err(anyhow!("slug {slug} not found in dead-letter"))
}

fn scan_for_existing(repo: &std::path::Path, slug: &str) -> Result<Option<std::path::PathBuf>> {
    for path in operator_common::iter_bug_files(repo) {
        if let Ok(raw) = std::fs::read_to_string(&path) {
            if let Ok(v) = serde_json::from_str::<serde_json::Value>(&raw) {
                if v.get("slug").and_then(|s| s.as_str()) == Some(slug) {
                    return Ok(Some(path));
                }
            }
        }
    }
    Ok(None)
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
