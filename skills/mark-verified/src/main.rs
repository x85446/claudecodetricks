use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use clap::Parser;
use serde_json::{json, Value};

#[derive(Parser, Debug)]
#[command(
    name = "mark-verified",
    about = "Rebase + ff-merge fix branch; transition to verified."
)]
struct Args {
    #[arg()]
    bug_id: String,
    #[arg(long, default_value = "from-chopper")]
    from_dir: String,
    #[arg(long, default_value_t = false)]
    as_human: bool,
    #[arg(long)]
    actor: Option<String>,
    #[arg(long)]
    target_repo_path: Option<PathBuf>,
    #[arg(long)]
    repo: Option<String>,
    #[arg(long)]
    fix_branch: Option<String>,
    #[arg(long, default_value = "main")]
    target_branch: String,
    #[arg(long, default_value_t = false)]
    dry_run: bool,
    #[arg(long)]
    cwd: Option<PathBuf>,
}

fn dry_run_enabled(arg: bool) -> bool {
    arg || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false)
}

fn agent_name() -> String {
    std::env::var("AGENT_NAME").unwrap_or_else(|_| "unknown-tester".to_string())
}

fn git_in(repo: &Path, args: &[&str]) -> Result<std::process::Output> {
    let out = Command::new("git")
        .args(args)
        .current_dir(repo)
        .output()
        .with_context(|| format!("spawn git {:?} in {}", args, repo.display()))?;
    Ok(out)
}

fn ensure_success(out: &std::process::Output, op: &str) -> Result<()> {
    if !out.status.success() {
        bail!(
            "{op} failed (exit {:?}): {}",
            out.status.code(),
            String::from_utf8_lossy(&out.stderr)
        );
    }
    Ok(())
}

fn rev_parse_head(repo: &Path) -> Result<String> {
    let out = git_in(repo, &["rev-parse", "HEAD"])?;
    ensure_success(&out, "git rev-parse HEAD")?;
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
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

fn run_git_mv(repo: &Path, from: &Path, to: &Path) -> Result<()> {
    let from_rel = from.strip_prefix(repo).unwrap_or(from);
    let to_rel = to.strip_prefix(repo).unwrap_or(to);
    if let Some(p) = to.parent() {
        fs::create_dir_all(p).ok();
    }
    let status = Command::new("git")
        .arg("mv")
        .arg(from_rel)
        .arg(to_rel)
        .current_dir(repo)
        .status()
        .with_context(|| format!("spawn git mv in {}", repo.display()))?;
    if !status.success() {
        bail!("git mv failed: {} -> {}", from.display(), to.display());
    }
    Ok(())
}

fn merge_fix_branch(repo: &Path, fix_branch: &str, target_branch: &str) -> Result<String> {
    ensure_success(&git_in(repo, &["fetch", "origin"])?, "git fetch origin")?;
    ensure_success(
        &git_in(repo, &["checkout", fix_branch])?,
        &format!("git checkout {fix_branch}"),
    )?;
    let rebase = git_in(
        repo,
        &["rebase", &format!("origin/{target_branch}"), fix_branch],
    )?;
    if !rebase.status.success() {
        // Abort the in-progress rebase to leave the working tree clean.
        let _ = git_in(repo, &["rebase", "--abort"]);
        bail!(
            "rebase onto origin/{target_branch} failed — call `update-bug --ghlstate needs_info` and let the coder iterate. stderr: {}",
            String::from_utf8_lossy(&rebase.stderr)
        );
    }
    ensure_success(
        &git_in(repo, &["checkout", target_branch])?,
        &format!("git checkout {target_branch}"),
    )?;
    ensure_success(
        &git_in(repo, &["merge", "--ff-only", fix_branch])?,
        &format!("git merge --ff-only {fix_branch}"),
    )?;
    let merge_sha = rev_parse_head(repo)?;
    ensure_success(
        &git_in(repo, &["push", "origin", target_branch])?,
        &format!("git push origin {target_branch}"),
    )?;
    let _ = git_in(repo, &["push", "origin", &format!(":{fix_branch}")]);
    let _ = git_in(repo, &["branch", "-d", fix_branch]);
    Ok(merge_sha)
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry_run = dry_run_enabled(args.dry_run);
    let cwd = args
        .cwd
        .clone()
        .unwrap_or_else(|| std::env::current_dir().expect("cwd"));

    let fix_branch = args
        .fix_branch
        .clone()
        .unwrap_or_else(|| format!("fix/{}", args.bug_id));
    let target_repo = match args.target_repo_path.clone() {
        Some(p) => p,
        None => {
            let r = args
                .repo
                .clone()
                .ok_or_else(|| anyhow::anyhow!("--repo or --target-repo-path required"))?;
            PathBuf::from(format!("/opt/repos/{r}"))
        }
    };

    let from_dir = cwd.join(&args.from_dir);
    let bug_path = from_dir.join(format!("{}.json", args.bug_id));
    if !bug_path.exists() {
        bail!("bug not found: {}", bug_path.display());
    }

    if dry_run {
        println!(
            "DRY_RUN: would merge {} -> {} in {} and transition {} -> verified",
            fix_branch,
            args.target_branch,
            target_repo.display(),
            args.bug_id
        );
        return Ok(());
    }

    if !target_repo.exists() {
        bail!("target repo path does not exist: {}", target_repo.display());
    }
    let merge_sha = merge_fix_branch(&target_repo, &fix_branch, &args.target_branch)?;

    let mut bug: Value = serde_json::from_str(&fs::read_to_string(&bug_path)?)?;
    let now = jiff::Timestamp::now().to_string();
    let by = agent_name();

    bug["ghlstate"] = json!("verified");
    bug["merge_commit"] = json!(merge_sha);
    bug["current_state"] = json!({"since": now, "note": "ff-merged"});

    let mut event = json!({
        "ts": now,
        "type": "verified",
        "by": by,
        "merge_commit": merge_sha,
    });
    if args.as_human {
        if let Some(actor) = &args.actor {
            event["actor"] = json!(actor);
        } else {
            event["actor"] = json!("human:unspecified");
        }
    } else if let Some(actor) = &args.actor {
        event["actor"] = json!(actor);
    }
    bug.as_object_mut()
        .unwrap()
        .entry("events")
        .or_insert_with(|| Value::Array(vec![]))
        .as_array_mut()
        .unwrap()
        .push(event);

    let tmp = bug_path.with_extension("json.tmp");
    fs::write(&tmp, serde_json::to_vec_pretty(&bug)?)?;
    fs::rename(&tmp, &bug_path)?;

    let to_path = cwd.join("to-chopper").join(format!("{}.json", args.bug_id));
    let repo = find_git_root(&cwd).unwrap_or(cwd.clone());
    run_git_mv(&repo, &bug_path, &to_path)?;

    println!("verified {} merge_commit={}", args.bug_id, merge_sha);
    Ok(())
}

#[cfg(test)]
mod tests {
    #[test]
    fn fix_branch_default() {
        let v = "BUG-000007";
        assert_eq!(format!("fix/{v}"), "fix/BUG-000007");
    }
}
