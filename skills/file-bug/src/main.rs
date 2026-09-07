use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use clap::Parser;
use serde_json::json;
use sha2::{Digest, Sha256};

#[derive(Parser, Debug)]
#[command(
    name = "file-bug",
    about = "Write a new slug to the leaf's to-chopper/."
)]
struct Args {
    #[arg(long)]
    title: String,
    #[arg(long)]
    description: String,
    #[arg(long)]
    validation: String,
    #[arg(long)]
    repo: String,
    #[arg(long)]
    priority: Option<String>,
    #[arg(long)]
    parent: Option<String>,
    #[arg(long = "blocked-by")]
    blocked_by: Vec<String>,
    #[arg(long)]
    submitter: Option<String>,
    #[arg(long, default_value_t = false)]
    dry_run: bool,
    #[arg(long)]
    cwd: Option<PathBuf>,
    /// Override target-repo crossbugs root (defaults to /opt/repos/<repo>/tests/bugs/crossbugs)
    #[arg(long)]
    crossbugs_root: Option<PathBuf>,
}

fn slugify(title: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(title.as_bytes());
    let digest = hasher.finalize();
    let hex_prefix = hex::encode(&digest[..6]);
    let normalized: String = title
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() {
                c.to_ascii_lowercase()
            } else {
                '-'
            }
        })
        .collect();
    let trimmed: String = normalized
        .split('-')
        .filter(|s| !s.is_empty())
        .take(6)
        .collect::<Vec<_>>()
        .join("-");
    if trimmed.is_empty() {
        format!("slug-{hex_prefix}")
    } else {
        format!("{trimmed}-{hex_prefix}")
    }
}

fn dry_run_enabled(arg: bool) -> bool {
    arg || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false)
}

fn write_atomic(path: &Path, contents: &[u8]) -> Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("path has no parent: {}", path.display()))?;
    fs::create_dir_all(parent).with_context(|| format!("create_dir_all {}", parent.display()))?;
    let tmp = parent.join(format!(
        ".{}.tmp",
        path.file_name().unwrap_or_default().to_string_lossy()
    ));
    fs::write(&tmp, contents).with_context(|| format!("write {}", tmp.display()))?;
    fs::rename(&tmp, path)
        .with_context(|| format!("rename {} -> {}", tmp.display(), path.display()))?;
    Ok(())
}

fn submitter_id() -> String {
    std::env::var("AGENT_NAME")
        .unwrap_or_else(|_| std::env::var("USER").unwrap_or_else(|_| "unknown-leaf".to_string()))
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry_run = dry_run_enabled(args.dry_run);
    let cwd = match args.cwd.clone() {
        Some(p) => p,
        None => std::env::current_dir().context("get cwd")?,
    };

    let slug = slugify(&args.title);
    let now = jiff::Timestamp::now().to_string();
    let submitter = args.submitter.clone().unwrap_or_else(submitter_id);

    let mut record = json!({
        "schema_version": 1,
        "ghlstate": "filed",
        "slug": slug,
        "title": args.title,
        "description": args.description,
        "validation": args.validation,
        "repo": args.repo,
        "filed_by": submitter,
        "filed_at": now,
        "bounce_count": 0,
        "source": "leaf",
    });

    if let Some(p) = &args.priority {
        record["priority"] = json!(p);
    }
    if let Some(parent) = &args.parent {
        record["parent"] = json!(parent);
    }
    if !args.blocked_by.is_empty() {
        record["blocked_by"] = json!(args.blocked_by);
    }

    let to_chopper = cwd.join("to-chopper");
    let slug_path = to_chopper.join(format!("{slug}.json"));
    let crossbugs_root = args
        .crossbugs_root
        .clone()
        .unwrap_or_else(|| PathBuf::from(format!("/opt/repos/{}/tests/bugs/crossbugs", args.repo)));
    let cross_md_path = crossbugs_root.join(format!("{slug}.md"));
    let cross_md = format!(
        "# {}\n\n## Description\n\n{}\n\n## Validation\n\n{}\n",
        args.title, args.description, args.validation
    );

    if dry_run {
        println!(
            "DRY_RUN: would write {} (slug={}) and {}",
            slug_path.display(),
            slug,
            cross_md_path.display()
        );
        return Ok(());
    }

    let serialized = serde_json::to_vec_pretty(&record)?;
    write_atomic(&slug_path, &serialized)?;
    // Crossbugs write is best-effort: target repo may not exist in test contexts.
    if crossbugs_root.exists()
        || std::env::var("FILE_BUG_FORCE_CROSSBUGS").ok().as_deref() == Some("1")
    {
        write_atomic(&cross_md_path, cross_md.as_bytes())?;
    }

    println!("filed slug={} path={}", slug, slug_path.display());
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn slug_is_deterministic() {
        let a = slugify("Login button does nothing on Safari");
        let b = slugify("Login button does nothing on Safari");
        assert_eq!(a, b);
        assert!(a.contains("login"));
    }

    #[test]
    fn slug_falls_back_when_title_has_no_alphanumerics() {
        let s = slugify("///");
        assert!(s.starts_with("slug-"));
    }
}
