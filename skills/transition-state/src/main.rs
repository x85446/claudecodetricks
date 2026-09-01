//! `transition-state` — the central GHLSTATE writer.
//!
//! Every chopper2-owned state change funnels through here so the state machine
//! has exactly one enforcement point:
//!   * consults `agent_comms_core::state::allowed_writers(s)` and refuses
//!     out-of-policy writes (non-zero exit);
//!   * increments `fix_attempts` on `needs_info → reassigned`;
//!   * increments `skill_retries[skill]` on skill error transitions;
//!   * triggers `escalation_needed` at `limits.bug_max_fix_attempts`;
//!   * triggers `skill_stuck` at `skills.on_error.max_retries`;
//!   * appends an `events[]` entry recording `{ts, by, action, reason}`.

use std::path::PathBuf;

use agent_comms_core::{cfg_load, clock::RealClock, state::GhlState, Clock};
use anyhow::{bail, Context, Result};
use clap::Parser;
use serde::Serialize;
use serde_json::{json, Value};

#[derive(Parser)]
#[command(name = "transition-state")]
struct Args {
    #[arg(long)]
    bug_path: PathBuf,
    #[arg(long)]
    new_state: String,
    #[arg(long)]
    actor: String,
    #[arg(long, default_value = "chopper2")]
    by_role: String,
    #[arg(long)]
    reason: Option<String>,
    #[arg(long)]
    skill: Option<String>,
    #[arg(long)]
    repo_root: Option<PathBuf>,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize)]
struct TransitionResult {
    prior_state: String,
    new_state: String,
    fix_attempts: u32,
    escalated: bool,
    stuck_skill: Option<String>,
}

fn allowed_for_role(role: &str, target: GhlState) -> bool {
    let writers = agent_comms_core::state::allowed_writers(target);
    writers.iter().any(|w| *w == role || *w == "*")
}

fn run(args: &Args) -> Result<TransitionResult> {
    let new_state = GhlState::parse(&args.new_state)
        .with_context(|| format!("unknown state: {}", args.new_state))?;
    if !allowed_for_role(&args.by_role, new_state) {
        bail!(
            "role `{}` not permitted to write state `{}`",
            args.by_role,
            new_state.as_str()
        );
    }

    let mut bug: Value =
        serde_json::from_slice(&std::fs::read(&args.bug_path).context("read bug")?)
            .context("parse bug json")?;

    let prior_str = bug
        .get("GHLSTATE")
        .and_then(Value::as_str)
        .or_else(|| bug.get("state").and_then(Value::as_str))
        .unwrap_or("filed")
        .to_string();
    let prior =
        GhlState::parse(&prior_str).with_context(|| format!("invalid prior state {prior_str}"))?;

    bug["GHLSTATE"] = json!(new_state.as_str());
    bug["state"] = json!(new_state.as_str());

    // §11 counter logic.
    let mut fix_attempts = bug.get("fix_attempts").and_then(Value::as_u64).unwrap_or(0) as u32;
    if matches!(prior, GhlState::NeedsInfo) && matches!(new_state, GhlState::Reassigned) {
        fix_attempts += 1;
    }
    bug["fix_attempts"] = json!(fix_attempts);

    let mut escalated = false;
    let mut stuck_skill: Option<String> = None;

    if let Some(repo_root) = &args.repo_root {
        let cfg = cfg_load::load_yaml(&repo_root.join("config.yml")).ok();
        let bug_max = cfg
            .as_ref()
            .and_then(|c| {
                c.pointer("/limits/defaults/bug_max_fix_attempts")
                    .and_then(Value::as_u64)
            })
            .unwrap_or(3) as u32;
        if fix_attempts >= bug_max {
            bug["GHLSTATE"] = json!(GhlState::EscalationNeeded.as_str());
            bug["state"] = json!(GhlState::EscalationNeeded.as_str());
            escalated = true;
        }

        // Skill retry tracking.
        if let Some(skill) = &args.skill {
            let entry_path = format!("/skill_retries/{}", skill);
            let cur = bug
                .pointer(&entry_path)
                .and_then(Value::as_u64)
                .unwrap_or(0) as u32;
            let next = cur + 1;
            bug.as_object_mut()
                .unwrap()
                .entry("skill_retries")
                .or_insert_with(|| json!({}));
            bug["skill_retries"][skill] = json!(next);
            let max_skill_retries = cfg
                .as_ref()
                .and_then(|c| {
                    c.pointer("/skills/defaults/on_error/max_retries")
                        .and_then(Value::as_u64)
                })
                .unwrap_or(3) as u32;
            if next >= max_skill_retries {
                bug["GHLSTATE"] = json!(GhlState::SkillStuck.as_str());
                bug["state"] = json!(GhlState::SkillStuck.as_str());
                stuck_skill = Some(skill.clone());
            }
        }
    }

    let clock = RealClock;
    let now_iso = clock.now_iso();
    let mut event = json!({
        "ts": now_iso.clone(),
        "by": args.actor.clone(),
        "action": format!("transition_to_{}", new_state.as_str()),
    });
    if let Some(reason) = &args.reason {
        event["reason"] = json!(reason);
    }
    let events_arr = bug
        .as_object_mut()
        .unwrap()
        .entry("events")
        .or_insert_with(|| json!([]));
    if let Some(arr) = events_arr.as_array_mut() {
        arr.push(event);
    }
    bug["updated_at"] = json!(now_iso);

    if !args.dry_run {
        let pretty = serde_json::to_vec_pretty(&bug)?;
        let tmp = args.bug_path.with_extension("json.tmp");
        std::fs::write(&tmp, &pretty).context("write tmp bug")?;
        std::fs::rename(&tmp, &args.bug_path).context("atomic rename")?;
    }

    Ok(TransitionResult {
        prior_state: prior.as_str().to_string(),
        new_state: bug["GHLSTATE"].as_str().unwrap_or("").to_string(),
        fix_attempts,
        escalated,
        stuck_skill,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let result = run(&args)?;
    println!("{}", serde_json::to_string(&result)?);
    Ok(())
}
