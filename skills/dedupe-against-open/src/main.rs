//! `dedupe-against-open` — keyword/Jaccard similarity scan against open and closed bugs.

use std::collections::HashSet;
use std::path::{Path, PathBuf};

use agent_comms_core::paths;
use anyhow::{Context, Result};
use clap::Parser;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use walkdir::WalkDir;

#[derive(Parser)]
#[command(name = "dedupe-against-open")]
struct Args {
    #[arg(long)]
    slug_path: PathBuf,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, default_value_t = 0.6)]
    threshold: f32,
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
enum MatchType {
    Open,
    Closed,
    None,
}

#[derive(Debug, Serialize, Deserialize)]
struct DedupeResult {
    match_type: MatchType,
    #[serde(skip_serializing_if = "Option::is_none")]
    match_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    score: Option<f32>,
}

fn tokenize(s: &str) -> HashSet<String> {
    s.to_lowercase()
        .split(|c: char| !c.is_alphanumeric())
        .filter(|t| t.len() > 2)
        .map(|t| t.to_string())
        .collect()
}

fn jaccard(a: &HashSet<String>, b: &HashSet<String>) -> f32 {
    if a.is_empty() && b.is_empty() {
        return 0.0;
    }
    let inter = a.intersection(b).count() as f32;
    let uni = a.union(b).count() as f32;
    inter / uni
}

fn extract_text(json: &Value) -> String {
    let title = json.get("title").and_then(Value::as_str).unwrap_or("");
    let desc = json
        .get("description")
        .and_then(Value::as_str)
        .unwrap_or("");
    format!("{title} {desc}")
}

fn scan_dir(dir: &Path, slug_tokens: &HashSet<String>, threshold: f32) -> Option<(String, f32)> {
    if !dir.exists() {
        return None;
    }
    let mut best: Option<(String, f32)> = None;
    for entry in WalkDir::new(dir)
        .max_depth(2)
        .into_iter()
        .filter_map(|e| e.ok())
    {
        let p = entry.path();
        if !p.is_file() {
            continue;
        }
        if p.extension().map(|e| e != "json").unwrap_or(true) {
            continue;
        }
        let stem = match p.file_stem().and_then(|s| s.to_str()) {
            Some(s) if s.starts_with("BUG-") => s.to_string(),
            _ => continue,
        };
        let bytes = match std::fs::read(p) {
            Ok(b) => b,
            Err(_) => continue,
        };
        let json: Value = match serde_json::from_slice(&bytes) {
            Ok(j) => j,
            Err(_) => continue,
        };
        let text = extract_text(&json);
        let tokens = tokenize(&text);
        let score = jaccard(slug_tokens, &tokens);
        if score >= threshold && best.as_ref().map(|(_, s)| score > *s).unwrap_or(true) {
            best = Some((stem, score));
        }
    }
    best
}

fn run(args: &Args) -> Result<DedupeResult> {
    let slug_bytes = std::fs::read(&args.slug_path)
        .with_context(|| format!("read slug at {}", args.slug_path.display()))?;
    let slug: Value = serde_json::from_slice(&slug_bytes).context("parse slug json")?;
    let slug_text = extract_text(&slug);
    let slug_tokens = tokenize(&slug_text);

    if let Some((id, score)) = scan_dir(
        &paths::bugs_dir(&args.repo_root),
        &slug_tokens,
        args.threshold,
    ) {
        return Ok(DedupeResult {
            match_type: MatchType::Open,
            match_id: Some(id),
            score: Some(score),
        });
    }
    if let Some((id, score)) = scan_dir(
        &paths::bugs_blocked_dir(&args.repo_root),
        &slug_tokens,
        args.threshold,
    ) {
        return Ok(DedupeResult {
            match_type: MatchType::Open,
            match_id: Some(id),
            score: Some(score),
        });
    }
    if let Some((id, score)) = scan_dir(
        &paths::bugs_closed_dir(&args.repo_root),
        &slug_tokens,
        args.threshold,
    ) {
        return Ok(DedupeResult {
            match_type: MatchType::Closed,
            match_id: Some(id),
            score: Some(score),
        });
    }
    Ok(DedupeResult {
        match_type: MatchType::None,
        match_id: None,
        score: None,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let result = run(&args)?;
    println!("{}", serde_json::to_string(&result)?);
    Ok(())
}
