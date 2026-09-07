//! `classify-incoming` — call Haiku to classify a filed slug.
//!
//! Behavior:
//!   1. Read slug JSON at `--slug-path`.
//!   2. Build a deterministic prompt from `title + description + validation`.
//!   3. Invoke `ClaudeRunner` with model `claude-haiku-4-5` and the JSON-schema
//!      response shape declared below.
//!   4. Reject with non-zero exit when `confidence < limits.dedupe_classifier_min_confidence`.
//!   5. Print `ClassifyResult` JSON to stdout for the caller (CLAUDE.md cycle).
//!
//! The trunk cycle script reads stdout and dispatches PATH A/B/C/D/E/F per §11.

use std::path::PathBuf;

use agent_comms_claude::{ClaudeRunner, ClaudeSubprocess};
use agent_comms_core::cfg_load;
use anyhow::{bail, Context, Result};
use clap::Parser;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

#[derive(Parser)]
#[command(name = "classify-incoming", about = "Classify a filed slug via Haiku")]
struct Args {
    #[arg(long)]
    slug_path: PathBuf,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct ClassifyResult {
    category: String,
    priority: String,
    repo: String,
    confidence: f32,
    #[serde(skip_serializing_if = "Option::is_none")]
    multi_repo: Option<Vec<RepoFraction>>,
}

#[derive(Debug, Serialize, Deserialize)]
struct RepoFraction {
    repo: String,
    confidence: f32,
}

fn classify_schema() -> Value {
    json!({
        "type": "object",
        "additionalProperties": false,
        "required": ["category", "priority", "repo", "confidence"],
        "properties": {
            "category":   { "type": "string" },
            "priority":   { "type": "string", "enum": ["P0","P1","P2","P3"] },
            "repo":       { "type": "string" },
            "confidence": { "type": "number", "minimum": 0.0, "maximum": 1.0 },
            "multi_repo": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": false,
                    "required": ["repo", "confidence"],
                    "properties": {
                        "repo":       { "type": "string" },
                        "confidence": { "type": "number", "minimum": 0.0, "maximum": 1.0 }
                    }
                }
            }
        }
    })
}

fn build_prompt(slug: &Value) -> String {
    let title = slug.get("title").and_then(Value::as_str).unwrap_or("");
    let description = slug
        .get("description")
        .and_then(Value::as_str)
        .unwrap_or("");
    let validation = slug.get("validation").and_then(Value::as_str).unwrap_or("");
    format!(
        "Classify this bug report.\n\nTitle: {title}\nDescription: {description}\nValidation: {validation}\n\nReturn JSON conforming to the response schema."
    )
}

fn cfg_f32(cfg: &Value, path: &[&str], default: f32) -> f32 {
    let mut node = cfg;
    for k in path {
        match node.get(k) {
            Some(v) => node = v,
            None => return default,
        }
    }
    node.as_f64().map(|x| x as f32).unwrap_or(default)
}

fn cfg_str<'a>(cfg: &'a Value, path: &[&str]) -> Option<&'a str> {
    let mut node = cfg;
    for k in path {
        node = node.get(k)?;
    }
    node.as_str()
}

fn run(args: &Args, runner: &dyn ClaudeRunner) -> Result<ClassifyResult> {
    let slug_bytes = std::fs::read(&args.slug_path)
        .with_context(|| format!("read slug at {}", args.slug_path.display()))?;
    let slug: Value = serde_json::from_slice(&slug_bytes).context("parse slug json")?;

    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("load config.yml")?;
    let threshold = cfg_f32(
        &cfg,
        &["limits", "defaults", "dedupe_classifier_min_confidence"],
        0.6,
    );
    let host = cfg_str(&cfg, &["cliproxyapi", "host"]).unwrap_or("chopper2-host");
    let proxy_port = cfg
        .get("cliproxyapi")
        .and_then(|c| c.get("proxies"))
        .and_then(|p| p.get("travis"))
        .and_then(|t| t.get("port"))
        .and_then(Value::as_u64)
        .unwrap_or(8081);
    let base_url = format!("http://{host}:{proxy_port}");

    let prompt = build_prompt(&slug);
    let schema = classify_schema();

    let raw = runner
        .invoke(&prompt, &schema, "claude-haiku-4-5", &base_url)
        .context("invoke claude haiku")?;
    let result: ClassifyResult = serde_json::from_value(raw).context("parse classify response")?;

    if result.confidence < threshold {
        bail!(
            "low confidence: {} < threshold {}",
            result.confidence,
            threshold
        );
    }
    Ok(result)
}

fn main() -> Result<()> {
    let args = Args::parse();
    let runner: Box<dyn ClaudeRunner> = Box::new(ClaudeSubprocess::new());
    let result = run(&args, runner.as_ref())?;
    println!("{}", serde_json::to_string(&result)?);
    Ok(())
}
