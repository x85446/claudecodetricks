use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;
use walkdir::WalkDir;

#[derive(Debug, Clone)]
pub struct Skill {
    pub name: String,
    pub description: String,
    pub src_path: PathBuf,
    pub last_activity: Option<SystemTime>,
}

impl Skill {
    pub fn relative_age(&self) -> String {
        match self.last_activity {
            Some(t) => age_string(t),
            None => String::from("?"),
        }
    }
}

fn age_string(t: SystemTime) -> String {
    let now = SystemTime::now();
    let secs = match now.duration_since(t) {
        Ok(d) => d.as_secs() as i64,
        Err(_) => return String::from("future"),
    };
    if secs < 60 {
        format!("{}s", secs)
    } else if secs < 3600 {
        format!("{}m", secs / 60)
    } else if secs < 86_400 {
        format!("{}h", secs / 3600)
    } else if secs < 86_400 * 30 {
        format!("{}d", secs / 86_400)
    } else if secs < 86_400 * 365 {
        format!("{}mo", secs / (86_400 * 30))
    } else {
        format!("{}y", secs / (86_400 * 365))
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProjectKind {
    Git,
    Claude,
}

#[derive(Debug, Clone)]
pub struct Project {
    pub relative: String,
    pub path: PathBuf,
    pub last_activity: Option<SystemTime>,
    pub kind: ProjectKind,
}

impl Project {
    pub fn company(&self) -> &str {
        match self.relative.split('/').next() {
            Some(c) => c,
            None => &self.relative,
        }
    }

    pub fn relative_age(&self) -> String {
        match self.last_activity {
            Some(t) => age_string(t),
            None => String::from("?"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Config {
    #[serde(default)]
    pub mappings: Vec<Mapping>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Mapping {
    pub skill: String,
    #[serde(default)]
    pub projects: Vec<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SyncStatus {
    NotMapped,
    NotInstalled,
    InSync,
    SrcNewer,
    DstNewer,
}

impl SyncStatus {
    pub fn badge(&self) -> &'static str {
        match self {
            SyncStatus::NotMapped => " [ ] ",
            SyncStatus::NotInstalled => " [✓] ",
            SyncStatus::InSync => " [✓] ",
            SyncStatus::SrcNewer => " [↑] ",
            SyncStatus::DstNewer => " [↓] ",
        }
    }
}

pub fn discover_skills(skills_root: &Path) -> Result<Vec<Skill>> {
    let mut out = Vec::new();
    let entries = fs::read_dir(skills_root)
        .with_context(|| format!("reading skills root {}", skills_root.display()))?;
    for entry in entries {
        let entry = entry?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        if name == "db" || name.starts_with('.') {
            continue;
        }
        let src_path = entry.path();
        let skill_md = src_path.join("SKILL.md");
        let description = if skill_md.exists() {
            parse_description(&skill_md).unwrap_or_else(|_| String::new())
        } else {
            String::new()
        };
        let last_activity = max_mtime(&src_path);
        out.push(Skill {
            name,
            description,
            src_path,
            last_activity,
        });
    }
    out.sort_by(|a, b| match (b.last_activity, a.last_activity) {
        (Some(bt), Some(at)) => bt.cmp(&at),
        (Some(_), None) => std::cmp::Ordering::Less,
        (None, Some(_)) => std::cmp::Ordering::Greater,
        (None, None) => a.name.cmp(&b.name),
    });
    Ok(out)
}

fn parse_description(skill_md: &Path) -> Result<String> {
    let text = fs::read_to_string(skill_md)?;
    let mut in_frontmatter = false;
    let mut desc = String::new();
    let mut continuation = false;
    for line in text.lines() {
        if line.trim() == "---" {
            if !in_frontmatter {
                in_frontmatter = true;
                continue;
            } else {
                break;
            }
        }
        if !in_frontmatter {
            continue;
        }
        if continuation {
            if line.starts_with(' ') || line.starts_with('\t') {
                desc.push(' ');
                desc.push_str(line.trim());
                continue;
            } else {
                continuation = false;
            }
        }
        if let Some(rest) = line.strip_prefix("description:") {
            desc = rest.trim().trim_matches('"').trim_matches('\'').to_string();
            continuation = true;
        }
    }
    Ok(desc)
}

pub fn discover_projects(workspace_root: &Path) -> Result<Vec<Project>> {
    let mut out = Vec::new();
    let max_depth = 6;
    let mut walker = WalkDir::new(workspace_root)
        .max_depth(max_depth)
        .follow_links(false)
        .into_iter();
    loop {
        let entry = match walker.next() {
            Some(Ok(e)) => e,
            Some(Err(_)) => continue,
            None => break,
        };
        if !entry.file_type().is_dir() {
            continue;
        }
        if entry.depth() == 0 {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        if name.starts_with('.')
            || matches!(
                name.as_str(),
                "node_modules" | "target" | "vendor" | "build" | "dist"
            )
        {
            walker.skip_current_dir();
            continue;
        }
        let has_git = entry.path().join(".git").exists();
        let has_claude = entry.path().join(".claude").is_dir();
        if has_git || has_claude {
            let path = entry.path().to_path_buf();
            let relative = match path.strip_prefix(workspace_root) {
                Ok(r) => r.to_string_lossy().to_string(),
                Err(_) => continue,
            };
            if !relative.is_empty() {
                let last_activity = if has_git {
                    git_activity(&path)
                } else {
                    fs::metadata(&path).ok().and_then(|m| m.modified().ok())
                };
                let kind = if has_git {
                    ProjectKind::Git
                } else {
                    ProjectKind::Claude
                };
                out.push(Project {
                    relative,
                    path,
                    last_activity,
                    kind,
                });
            }
            if has_git {
                walker.skip_current_dir();
            }
        }
    }
    out.sort_by(|a, b| match (b.last_activity, a.last_activity) {
        (Some(bt), Some(at)) => bt.cmp(&at),
        (Some(_), None) => std::cmp::Ordering::Less,
        (None, Some(_)) => std::cmp::Ordering::Greater,
        (None, None) => a.relative.cmp(&b.relative),
    });
    Ok(out)
}

fn git_activity(project_path: &Path) -> Option<SystemTime> {
    let candidates = [
        ".git/HEAD",
        ".git/index",
        ".git/FETCH_HEAD",
        ".git/ORIG_HEAD",
        ".git/COMMIT_EDITMSG",
    ];
    let mut latest: Option<SystemTime> = None;
    for rel in candidates {
        let p = project_path.join(rel);
        if let Ok(meta) = fs::metadata(&p) {
            if let Ok(mt) = meta.modified() {
                latest = Some(match latest {
                    Some(cur) if cur >= mt => cur,
                    _ => mt,
                });
            }
        }
    }
    if latest.is_some() {
        return latest;
    }
    fs::metadata(project_path)
        .ok()
        .and_then(|m| m.modified().ok())
}

pub fn skill_dst_path(project: &Project, skill_name: &str) -> PathBuf {
    project.path.join(".claude").join("skills").join(skill_name)
}

pub fn max_mtime(path: &Path) -> Option<SystemTime> {
    if !path.exists() {
        return None;
    }
    let mut latest: Option<SystemTime> = None;
    for entry in WalkDir::new(path).into_iter().flatten() {
        if let Ok(meta) = entry.metadata() {
            if let Ok(mt) = meta.modified() {
                latest = Some(match latest {
                    Some(cur) if cur >= mt => cur,
                    _ => mt,
                });
            }
        }
    }
    latest
}

pub fn compute_status(skill: &Skill, project: &Project, mapped: bool) -> SyncStatus {
    if !mapped {
        return SyncStatus::NotMapped;
    }
    let dst = skill_dst_path(project, &skill.name);
    if !dst.exists() {
        return SyncStatus::NotInstalled;
    }
    if dir_identical(&skill.src_path, &dst) {
        return SyncStatus::InSync;
    }
    let src_mt = max_mtime(&skill.src_path);
    let dst_mt = max_mtime(&dst);
    match (src_mt, dst_mt) {
        (Some(s), Some(d)) if s >= d => SyncStatus::SrcNewer,
        (Some(_), Some(_)) => SyncStatus::DstNewer,
        _ => SyncStatus::SrcNewer,
    }
}

fn is_noise(name: &str) -> bool {
    matches!(
        name,
        ".DS_Store" | "Thumbs.db" | ".AppleDouble" | ".localized"
    ) || name.starts_with("._")
}

fn collect_files(root: &Path) -> Option<Vec<(PathBuf, u64)>> {
    let mut out: Vec<(PathBuf, u64)> = Vec::new();
    for entry in WalkDir::new(root).into_iter().flatten() {
        if !entry.file_type().is_file() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        if is_noise(&name) {
            continue;
        }
        let rel = entry.path().strip_prefix(root).ok()?.to_path_buf();
        let len = entry.metadata().ok()?.len();
        out.push((rel, len));
    }
    out.sort_by(|a, b| a.0.cmp(&b.0));
    Some(out)
}

pub fn dir_identical(a: &Path, b: &Path) -> bool {
    let a_files = match collect_files(a) {
        Some(v) => v,
        None => return false,
    };
    let b_files = match collect_files(b) {
        Some(v) => v,
        None => return false,
    };
    if a_files.len() != b_files.len() {
        return false;
    }
    for (i, (rel_a, size_a)) in a_files.iter().enumerate() {
        let (rel_b, size_b) = &b_files[i];
        if rel_a != rel_b || size_a != size_b {
            return false;
        }
    }
    for (rel, _) in &a_files {
        let pa = a.join(rel);
        let pb = b.join(rel);
        let ba = match fs::read(&pa) {
            Ok(v) => v,
            Err(_) => return false,
        };
        let bb = match fs::read(&pb) {
            Ok(v) => v,
            Err(_) => return false,
        };
        if ba != bb {
            return false;
        }
    }
    true
}

pub fn copy_skill(src: &Path, dst: &Path) -> Result<usize> {
    if !src.is_dir() {
        anyhow::bail!("source not a directory: {}", src.display());
    }
    if dst.exists() {
        fs::remove_dir_all(dst)
            .with_context(|| format!("removing existing dst {}", dst.display()))?;
    }
    fs::create_dir_all(dst).with_context(|| format!("creating dst {}", dst.display()))?;
    let mut count = 0usize;
    for entry in WalkDir::new(src).into_iter().flatten() {
        let rel = match entry.path().strip_prefix(src) {
            Ok(r) => r,
            Err(_) => continue,
        };
        if rel.as_os_str().is_empty() {
            continue;
        }
        let target = dst.join(rel);
        if entry.file_type().is_dir() {
            fs::create_dir_all(&target)?;
        } else if entry.file_type().is_file() {
            if let Some(parent) = target.parent() {
                fs::create_dir_all(parent)?;
            }
            fs::copy(entry.path(), &target).with_context(|| {
                format!("copy {} -> {}", entry.path().display(), target.display())
            })?;
            count += 1;
        }
    }
    Ok(count)
}

impl Config {
    pub fn load(path: &Path) -> Result<Self> {
        if !path.exists() {
            return Ok(Self::default());
        }
        let text = fs::read_to_string(path)
            .with_context(|| format!("reading config {}", path.display()))?;
        let cfg: Config = toml::from_str(&text)
            .with_context(|| format!("parsing config {}", path.display()))?;
        Ok(cfg)
    }

    pub fn save(&self, path: &Path) -> Result<()> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)?;
        }
        let text = toml::to_string_pretty(self)?;
        fs::write(path, text).with_context(|| format!("writing config {}", path.display()))?;
        Ok(())
    }

    pub fn is_mapped(&self, skill: &str, project_rel: &str) -> bool {
        self.mappings
            .iter()
            .find(|m| m.skill == skill)
            .map(|m| m.projects.iter().any(|p| p == project_rel))
            .unwrap_or(false)
    }

    pub fn toggle(&mut self, skill: &str, project_rel: &str) {
        if let Some(m) = self.mappings.iter_mut().find(|m| m.skill == skill) {
            if let Some(idx) = m.projects.iter().position(|p| p == project_rel) {
                m.projects.remove(idx);
            } else {
                m.projects.push(project_rel.to_string());
                m.projects.sort();
            }
            if m.projects.is_empty() {
                let pos = self.mappings.iter().position(|x| x.skill == skill).unwrap();
                self.mappings.remove(pos);
            }
        } else {
            self.mappings.push(Mapping {
                skill: skill.to_string(),
                projects: vec![project_rel.to_string()],
            });
            self.mappings.sort_by(|a, b| a.skill.cmp(&b.skill));
        }
    }

    pub fn projects_for(&self, skill: &str) -> Vec<String> {
        self.mappings
            .iter()
            .find(|m| m.skill == skill)
            .map(|m| m.projects.clone())
            .unwrap_or_default()
    }
}
