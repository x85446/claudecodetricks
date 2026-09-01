//! dump-state — Operator skill (§9, IT-S32, AC32).
use std::path::PathBuf;

use agent_comms_core::paths::{identities_path, notify_cooldowns_path};
use anyhow::Result;
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "dump-state")]
struct Args {
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };

    let bugs: Vec<serde_json::Value> = operator_common::iter_bug_files(&repo)
        .filter_map(|p| std::fs::read_to_string(&p).ok())
        .filter_map(|raw| serde_json::from_str(&raw).ok())
        .collect();

    let slugs: Vec<serde_json::Value> = operator_common::iter_slug_files(&repo)
        .filter_map(|p| std::fs::read_to_string(&p).ok())
        .filter_map(|raw| serde_json::from_str(&raw).ok())
        .collect();

    let mut host_state = serde_json::Map::new();
    let host_dir = repo.join("infra/host-state");
    if host_dir.exists() {
        for entry in walkdir::WalkDir::new(&host_dir)
            .max_depth(1)
            .into_iter()
            .flatten()
        {
            if !entry.file_type().is_file() {
                continue;
            }
            let stem = match entry.path().file_stem().and_then(|s| s.to_str()) {
                Some(s) => s.to_string(),
                None => continue,
            };
            if let Ok(raw) = std::fs::read_to_string(entry.path()) {
                if let Ok(v) = serde_json::from_str::<serde_json::Value>(&raw) {
                    host_state.insert(stem, v);
                }
            }
        }
    }

    let identities: serde_json::Value = std::fs::read_to_string(identities_path(&repo))
        .ok()
        .and_then(|s| serde_json::from_str(&s).ok())
        .unwrap_or(serde_json::json!({}));

    let cooldowns: serde_json::Value = std::fs::read_to_string(notify_cooldowns_path(&repo))
        .ok()
        .and_then(|s| serde_json::from_str(&s).ok())
        .unwrap_or(serde_json::json!({}));

    let config_yaml: serde_json::Value = std::fs::read_to_string(repo.join("config.yml"))
        .ok()
        .and_then(|s| serde_yaml::from_str(&s).ok())
        .unwrap_or(serde_json::json!({}));

    let snapshot = serde_json::json!({
        "schema_version": 1,
        "generated_at": jiff::Timestamp::now().to_string(),
        "bugs": bugs,
        "slugs": slugs,
        "host_state": host_state,
        "identities": identities,
        "notify_cooldowns": cooldowns,
        "config": config_yaml,
    });
    println!("{}", serde_json::to_string_pretty(&snapshot)?);
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
