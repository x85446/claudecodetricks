//! `health-report` — aggregate host health, stale bugs, manifest drift.

use std::path::PathBuf;

use agent_comms_core::{cfg_load, paths};
use anyhow::{Context, Result};
use clap::Parser;
use jiff::Timestamp;
use serde::Serialize;
use serde_json::Value;
use walkdir::WalkDir;

#[derive(Parser)]
#[command(name = "health-report")]
struct Args {
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long)]
    persist: bool,
}

#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "lowercase")]
enum Severity {
    Ok,
    Warn,
    Err,
}

#[derive(Debug, Serialize)]
struct HealthItem {
    severity: Severity,
    category: String,
    detail: String,
}

#[derive(Debug, Serialize)]
struct HealthReport {
    items: Vec<HealthItem>,
    written_path: Option<PathBuf>,
}

fn check_stale_in_progress(repo_root: &std::path::Path, warn_hours: u32) -> Vec<HealthItem> {
    let mut out = Vec::new();
    let dir = paths::bugs_dir(repo_root);
    if !dir.exists() {
        return out;
    }
    let now = Timestamp::now();
    let warn_secs = (warn_hours as i64) * 3600;
    for entry in WalkDir::new(&dir)
        .max_depth(1)
        .into_iter()
        .filter_map(|e| e.ok())
    {
        if !entry.file_type().is_file() {
            continue;
        }
        let bytes = match std::fs::read(entry.path()) {
            Ok(b) => b,
            Err(_) => continue,
        };
        let bug: Value = match serde_json::from_slice(&bytes) {
            Ok(v) => v,
            Err(_) => continue,
        };
        let state = bug
            .get("GHLSTATE")
            .or_else(|| bug.get("state"))
            .and_then(Value::as_str)
            .unwrap_or("");
        if state != "in_progress" {
            continue;
        }
        if let Some(updated_at) = bug.get("updated_at").and_then(Value::as_str) {
            if let Ok(t) = updated_at.parse::<Timestamp>() {
                if (now.as_second() - t.as_second()) > warn_secs {
                    let id = bug
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or("?")
                        .to_string();
                    out.push(HealthItem {
                        severity: Severity::Warn,
                        category: "stale_in_progress".to_string(),
                        detail: format!("bug {id} stale > {warn_hours}h"),
                    });
                }
            }
        }
    }
    out
}

fn check_host_silence(repo_root: &std::path::Path, cycle_minutes: u32) -> Vec<HealthItem> {
    let mut out = Vec::new();
    let dir = repo_root.join("infra/health");
    if !dir.exists() {
        return out;
    }
    let now = Timestamp::now();
    let warn_secs = (cycle_minutes as i64) * 2 * 60; // 2× cadence
    for entry in WalkDir::new(&dir)
        .max_depth(1)
        .into_iter()
        .filter_map(|e| e.ok())
    {
        let p = entry.path();
        if !p.is_file() {
            continue;
        }
        let host = match p.file_stem().and_then(|s| s.to_str()) {
            Some(s) => s.to_string(),
            None => continue,
        };
        let bytes = match std::fs::read(p) {
            Ok(b) => b,
            Err(_) => continue,
        };
        let v: Value = match serde_json::from_slice(&bytes) {
            Ok(v) => v,
            Err(_) => continue,
        };
        let last = v
            .get("last_seen")
            .or_else(|| v.get("ts"))
            .and_then(Value::as_str);
        if let Some(ts) = last {
            if let Ok(t) = ts.parse::<Timestamp>() {
                if (now.as_second() - t.as_second()) > warn_secs {
                    out.push(HealthItem {
                        severity: Severity::Err,
                        category: "manifest_drift".to_string(),
                        detail: format!("host {host} silent > {} min", warn_secs / 60),
                    });
                }
            }
        }
    }
    out
}

fn check_dr_backup(cfg: &Value) -> Vec<HealthItem> {
    let dr_s3 = cfg
        .pointer("/dr/backup_s3")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    if !dr_s3 {
        vec![HealthItem {
            severity: Severity::Warn,
            category: "dr_backup_s3".to_string(),
            detail: "off-site S3 replication not configured".to_string(),
        }]
    } else {
        Vec::new()
    }
}

fn run(args: &Args) -> Result<HealthReport> {
    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("load config.yml")?;
    let warn_hours = cfg
        .get("stale")
        .and_then(|v| v.get("defaults"))
        .and_then(|v| v.get("in_progress_warn_hours"))
        .and_then(Value::as_u64)
        .unwrap_or(24) as u32;
    let cycle_minutes = cfg
        .get("cron")
        .and_then(|v| v.get("defaults"))
        .and_then(|v| v.get("chopper2"))
        .and_then(|v| v.get("every_minutes"))
        .and_then(Value::as_u64)
        .unwrap_or(1) as u32;

    let mut items = Vec::new();
    items.extend(check_stale_in_progress(&args.repo_root, warn_hours));
    items.extend(check_host_silence(&args.repo_root, cycle_minutes));
    items.extend(check_dr_backup(&cfg));
    if items.is_empty() {
        items.push(HealthItem {
            severity: Severity::Ok,
            category: "ok".to_string(),
            detail: "all checks green".to_string(),
        });
    }

    let written = if args.persist {
        let date = Timestamp::now().to_string()[..10].to_string();
        let dir = args.repo_root.join("agents/chopper2/reports");
        std::fs::create_dir_all(&dir).ok();
        let path = dir.join(format!("{date}.json"));
        std::fs::write(&path, serde_json::to_vec_pretty(&items)?)?;
        Some(path)
    } else {
        None
    };

    Ok(HealthReport {
        items,
        written_path: written,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let r = run(&args)?;
    println!("{}", serde_json::to_string(&r)?);
    Ok(())
}
