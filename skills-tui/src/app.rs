use crate::model::{
    compute_status, copy_skill, dir_identical, discover_projects, discover_skills, skill_dst_path,
    Config, Project, Skill, SyncStatus,
};
use anyhow::Result;
use std::path::PathBuf;
use std::time::{Duration, Instant};

const SORT_MENU_LINGER: Duration = Duration::from_secs(1);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Focus {
    Skills,
    Projects,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SortKey {
    Alpha,
    Age,
    Popularity,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SortDir {
    Asc,
    Desc,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SortOrder {
    pub key: SortKey,
    pub dir: SortDir,
}

impl SortOrder {
    pub const fn new(key: SortKey, dir: SortDir) -> Self {
        Self { key, dir }
    }

    pub fn label(&self) -> String {
        let key = match self.key {
            SortKey::Alpha => "a-z",
            SortKey::Age => "age",
            SortKey::Popularity => "pop",
        };
        let dir = match self.dir {
            SortDir::Asc => "↑",
            SortDir::Desc => "↓",
        };
        format!("{}{}", key, dir)
    }

    pub fn long_label(&self) -> &'static str {
        match (self.key, self.dir) {
            (SortKey::Alpha, SortDir::Asc) => "alpha asc (A→Z)",
            (SortKey::Alpha, SortDir::Desc) => "alpha desc (Z→A)",
            (SortKey::Age, SortDir::Asc) => "age asc (oldest first)",
            (SortKey::Age, SortDir::Desc) => "age desc (newest first)",
            (SortKey::Popularity, SortDir::Asc) => "popularity asc (least → most)",
            (SortKey::Popularity, SortDir::Desc) => "popularity desc (most → least)",
        }
    }
}

pub const SORT_OPTIONS: &[(char, SortOrder)] = &[
    ('a', SortOrder::new(SortKey::Alpha, SortDir::Asc)),
    ('A', SortOrder::new(SortKey::Alpha, SortDir::Desc)),
    ('g', SortOrder::new(SortKey::Age, SortDir::Asc)),
    ('G', SortOrder::new(SortKey::Age, SortDir::Desc)),
    ('p', SortOrder::new(SortKey::Popularity, SortDir::Asc)),
    ('P', SortOrder::new(SortKey::Popularity, SortDir::Desc)),
];

pub enum InputMode {
    Normal,
    Filter,
    Confirm(ConfirmAction),
    CompanyMenu,
    SortMenu,
    Help,
}

#[derive(Debug, Clone)]
pub enum ConfirmAction {
    PushAll,
    PullSelected,
}

pub struct App {
    pub workspace_root: PathBuf,
    pub skills_root: PathBuf,
    pub config_path: PathBuf,
    pub global_skills_root: PathBuf,

    pub skills: Vec<Skill>,
    pub projects: Vec<Project>,
    pub config: Config,

    pub focus: Focus,
    pub mode: InputMode,
    pub filter: String,
    pub project_filter: String,
    pub company_filter: Option<String>,
    pub mapped_only: bool,
    pub skill_sort: SortOrder,
    pub project_sort: SortOrder,
    pub sort_menu_cursor: usize,
    pub sort_menu_close_at: Option<Instant>,

    pub skill_idx: usize,
    pub project_idx: usize,

    pub status: String,
    pub should_quit: bool,
}

impl App {
    pub fn new(
        workspace_root: PathBuf,
        skills_root: PathBuf,
        config_path: PathBuf,
        global_skills_root: PathBuf,
    ) -> Result<Self> {
        let skills = discover_skills(&skills_root)?;
        let projects = discover_projects(&workspace_root)?;
        let config = Config::load(&config_path)?;
        Ok(Self {
            workspace_root,
            skills_root,
            config_path,
            global_skills_root,
            skills,
            projects,
            config,
            focus: Focus::Skills,
            mode: InputMode::Normal,
            filter: String::new(),
            project_filter: String::new(),
            company_filter: None,
            mapped_only: true,
            skill_sort: SortOrder::new(SortKey::Popularity, SortDir::Desc),
            project_sort: SortOrder::new(SortKey::Age, SortDir::Desc),
            sort_menu_cursor: 0,
            sort_menu_close_at: None,
            skill_idx: 0,
            project_idx: 0,
            status: String::from("Ready"),
            should_quit: false,
        })
    }

    pub fn refresh(&mut self) -> Result<()> {
        self.skills = discover_skills(&self.skills_root)?;
        self.projects = discover_projects(&self.workspace_root)?;
        self.config = Config::load(&self.config_path)?;
        self.apply_sort();
        let visible_skills = self.filtered_skills().len();
        let visible_projects = self.visible_projects().len();
        self.skill_idx = self.skill_idx.min(visible_skills.saturating_sub(1));
        self.project_idx = self.project_idx.min(visible_projects.saturating_sub(1));
        self.status = format!(
            "Refreshed: {} skills, {} projects",
            self.skills.len(),
            self.projects.len()
        );
        Ok(())
    }

    pub fn apply_sort(&mut self) {
        let skill_pop: Vec<usize> = self
            .skills
            .iter()
            .map(|s| self.config.projects_for(&s.name).len())
            .collect();
        let mut indexed: Vec<(usize, _)> = self
            .skills
            .iter()
            .cloned()
            .enumerate()
            .map(|(i, s)| (skill_pop[i], s))
            .collect();
        let order = self.skill_sort;
        indexed.sort_by(|a, b| {
            let cmp = match order.key {
                SortKey::Alpha => a.1.name.cmp(&b.1.name),
                SortKey::Age => match (a.1.last_activity, b.1.last_activity) {
                    (Some(at), Some(bt)) => at.cmp(&bt),
                    (Some(_), None) => std::cmp::Ordering::Greater,
                    (None, Some(_)) => std::cmp::Ordering::Less,
                    (None, None) => a.1.name.cmp(&b.1.name),
                },
                SortKey::Popularity => a.0.cmp(&b.0).then_with(|| a.1.name.cmp(&b.1.name)),
            };
            match order.dir {
                SortDir::Asc => cmp,
                SortDir::Desc => cmp.reverse(),
            }
        });
        self.skills = indexed.into_iter().map(|(_, s)| s).collect();

        let proj_pop: Vec<usize> = self
            .projects
            .iter()
            .map(|p| {
                self.config
                    .mappings
                    .iter()
                    .filter(|m| m.projects.iter().any(|x| x == &p.relative))
                    .count()
            })
            .collect();
        let mut indexed: Vec<(usize, _)> = self
            .projects
            .iter()
            .cloned()
            .enumerate()
            .map(|(i, p)| (proj_pop[i], p))
            .collect();
        let order = self.project_sort;
        indexed.sort_by(|a, b| {
            let cmp = match order.key {
                SortKey::Alpha => a.1.relative.cmp(&b.1.relative),
                SortKey::Age => match (a.1.last_activity, b.1.last_activity) {
                    (Some(at), Some(bt)) => at.cmp(&bt),
                    (Some(_), None) => std::cmp::Ordering::Greater,
                    (None, Some(_)) => std::cmp::Ordering::Less,
                    (None, None) => a.1.relative.cmp(&b.1.relative),
                },
                SortKey::Popularity => a.0.cmp(&b.0).then_with(|| a.1.relative.cmp(&b.1.relative)),
            };
            match order.dir {
                SortDir::Asc => cmp,
                SortDir::Desc => cmp.reverse(),
            }
        });
        self.projects = indexed.into_iter().map(|(_, p)| p).collect();
    }

    pub fn open_sort_menu(&mut self) {
        let current = match self.focus {
            Focus::Skills => self.skill_sort,
            Focus::Projects => self.project_sort,
        };
        self.sort_menu_cursor = SORT_OPTIONS
            .iter()
            .position(|(_, o)| *o == current)
            .unwrap_or(0);
        self.sort_menu_close_at = None;
        self.mode = InputMode::SortMenu;
    }

    pub fn close_sort_menu(&mut self) {
        self.mode = InputMode::Normal;
        self.sort_menu_close_at = None;
    }

    pub fn sort_menu_move(&mut self, delta: i32) {
        let len = SORT_OPTIONS.len() as i32;
        if len == 0 {
            return;
        }
        self.sort_menu_cursor = (self.sort_menu_cursor as i32 + delta).rem_euclid(len) as usize;
        self.sort_menu_close_at = None;
    }

    pub fn sort_menu_apply_cursor(&mut self) {
        let (_, order) = SORT_OPTIONS[self.sort_menu_cursor];
        self.set_sort(order);
        self.sort_menu_close_at = Some(Instant::now() + SORT_MENU_LINGER);
    }

    pub fn sort_menu_advance(&mut self) {
        self.sort_menu_move(1);
        self.sort_menu_apply_cursor();
    }

    pub fn sort_menu_jump(&mut self, key: char) -> bool {
        if let Some(idx) = SORT_OPTIONS.iter().position(|(ch, _)| *ch == key) {
            self.sort_menu_cursor = idx;
            self.sort_menu_apply_cursor();
            true
        } else {
            false
        }
    }

    pub fn tick(&mut self) {
        if let InputMode::SortMenu = self.mode {
            if let Some(close_at) = self.sort_menu_close_at {
                if Instant::now() >= close_at {
                    self.close_sort_menu();
                }
            }
        }
    }

    pub fn set_sort(&mut self, order: SortOrder) {
        match self.focus {
            Focus::Skills => {
                self.skill_sort = order;
                self.skill_idx = 0;
                self.status = format!("skill sort → {}", order.long_label());
            }
            Focus::Projects => {
                self.project_sort = order;
                self.project_idx = 0;
                self.status = format!("project sort → {}", order.long_label());
            }
        }
        self.apply_sort();
    }

    pub fn filtered_skills(&self) -> Vec<usize> {
        if self.filter.is_empty() {
            return (0..self.skills.len()).collect();
        }
        let f = self.filter.to_lowercase();
        self.skills
            .iter()
            .enumerate()
            .filter(|(_, s)| {
                s.name.to_lowercase().contains(&f)
                    || s.description.to_lowercase().contains(&f)
            })
            .map(|(i, _)| i)
            .collect()
    }

    pub fn current_skill(&self) -> Option<&Skill> {
        let visible = self.filtered_skills();
        visible.get(self.skill_idx).and_then(|i| self.skills.get(*i))
    }

    pub fn visible_projects(&self) -> Vec<usize> {
        let pf = self.project_filter.to_lowercase();
        self.projects
            .iter()
            .enumerate()
            .filter(|(_, p)| {
                let company_ok = match &self.company_filter {
                    Some(c) => p.company() == c.as_str(),
                    None => true,
                };
                let mapped_ok = if self.mapped_only {
                    self.config
                        .mappings
                        .iter()
                        .any(|m| m.projects.iter().any(|x| x == &p.relative))
                } else {
                    true
                };
                let text_ok = if pf.is_empty() {
                    true
                } else {
                    p.relative.to_lowercase().contains(&pf)
                };
                company_ok && mapped_ok && text_ok
            })
            .map(|(i, _)| i)
            .collect()
    }

    pub fn set_mapped_only(&mut self, on: bool) {
        if self.mapped_only != on {
            self.mapped_only = on;
            self.project_idx = 0;
            self.status = if on {
                "show: only projects with at least one skill mapped".to_string()
            } else {
                "show: all projects".to_string()
            };
        }
    }

    pub fn current_project(&self) -> Option<&Project> {
        let visible = self.visible_projects();
        visible
            .get(self.project_idx)
            .and_then(|i| self.projects.get(*i))
    }

    pub fn move_skill(&mut self, delta: i32) {
        let visible = self.filtered_skills();
        if visible.is_empty() {
            return;
        }
        let new = (self.skill_idx as i32 + delta)
            .rem_euclid(visible.len() as i32) as usize;
        self.skill_idx = new;
    }

    pub fn move_project(&mut self, delta: i32) {
        let visible = self.visible_projects();
        if visible.is_empty() {
            return;
        }
        let new = (self.project_idx as i32 + delta)
            .rem_euclid(visible.len() as i32) as usize;
        self.project_idx = new;
    }

    pub fn companies(&self) -> Vec<(String, char)> {
        let mut seen: Vec<String> = Vec::new();
        for p in &self.projects {
            let c = p.company().to_string();
            if !seen.contains(&c) {
                seen.push(c);
            }
        }
        seen.sort();
        let mut taken: Vec<char> = Vec::new();
        let mut out: Vec<(String, char)> = Vec::with_capacity(seen.len());
        for name in &seen {
            let mut chosen: Option<char> = None;
            for ch in name.chars() {
                let lc = ch.to_ascii_lowercase();
                if !lc.is_ascii_alphanumeric() {
                    continue;
                }
                if !taken.contains(&lc) {
                    taken.push(lc);
                    chosen = Some(lc);
                    break;
                }
            }
            let ch = chosen.unwrap_or_else(|| name.chars().next().unwrap_or('?'));
            out.push((name.clone(), ch));
        }
        out
    }

    pub fn set_company_filter(&mut self, company: Option<String>) {
        self.company_filter = company;
        self.project_idx = 0;
    }

    pub fn toggle_mapping(&mut self) -> Result<()> {
        let skill = match self.current_skill() {
            Some(s) => s.clone(),
            None => return Ok(()),
        };
        let project = match self.current_project() {
            Some(p) => p.clone(),
            None => return Ok(()),
        };
        let was_mapped = self.config.is_mapped(&skill.name, &project.relative);
        self.config.toggle(&skill.name, &project.relative);
        self.config.save(&self.config_path)?;
        if was_mapped {
            self.status = format!(
                "✗ unmapped {} from {} (destination files left in place)",
                skill.name, project.relative
            );
        } else {
            let dst = skill_dst_path(&project, &skill.name);
            if !dst.exists() {
                self.status = format!(
                    "✓ mapped {} → {} — destination missing, press i to push",
                    skill.name, project.relative
                );
            } else if dir_identical(&skill.src_path, &dst) {
                self.status = format!(
                    "✓ mapped {} → {} — destination already byte-identical (no push needed)",
                    skill.name, project.relative
                );
            } else {
                self.status = format!(
                    "✓ mapped {} → {} — differs, press i to push or r to pull",
                    skill.name, project.relative
                );
            }
        }
        Ok(())
    }

    pub fn push_selected(&mut self) -> Result<()> {
        let skill = match self.current_skill() {
            Some(s) => s.clone(),
            None => return Ok(()),
        };
        let project = match self.current_project() {
            Some(p) => p.clone(),
            None => return Ok(()),
        };
        let dst = skill_dst_path(&project, &skill.name);
        match copy_skill(&skill.src_path, &dst) {
            Ok(n) => {
                self.status = format!("✔ pushed {} → {} ({} files)", skill.name, project.relative, n);
            }
            Err(e) => {
                self.status = format!("✘ push failed: {}", e);
            }
        }
        Ok(())
    }

    pub fn pull_selected(&mut self) -> Result<()> {
        let skill = match self.current_skill() {
            Some(s) => s.clone(),
            None => return Ok(()),
        };
        let project = match self.current_project() {
            Some(p) => p.clone(),
            None => return Ok(()),
        };
        let dst = skill_dst_path(&project, &skill.name);
        if !dst.exists() {
            self.status = format!("✘ nothing to pull: {} not at {}", skill.name, dst.display());
            return Ok(());
        }
        match copy_skill(&dst, &skill.src_path) {
            Ok(n) => {
                self.status = format!("✔ pulled {} ← {} ({} files)", skill.name, project.relative, n);
            }
            Err(e) => {
                self.status = format!("✘ pull failed: {}", e);
            }
        }
        Ok(())
    }

    pub fn push_global(&mut self) -> Result<()> {
        let skill = match self.current_skill() {
            Some(s) => s.clone(),
            None => return Ok(()),
        };
        let dst = self.global_skills_root.join(&skill.name);
        match copy_skill(&skill.src_path, &dst) {
            Ok(n) => {
                self.status = format!(
                    "✔ pushed {} → {} ({} files, global)",
                    skill.name,
                    dst.display(),
                    n
                );
            }
            Err(e) => {
                self.status = format!("✘ global push failed: {}", e);
            }
        }
        Ok(())
    }

    pub fn push_all(&mut self) -> Result<()> {
        let mut total = 0usize;
        let mut errs: Vec<String> = Vec::new();
        let mappings = self.config.mappings.clone();
        for m in &mappings {
            let skill = match self.skills.iter().find(|s| s.name == m.skill) {
                Some(s) => s,
                None => continue,
            };
            for proj_rel in &m.projects {
                let project = match self.projects.iter().find(|p| &p.relative == proj_rel) {
                    Some(p) => p,
                    None => {
                        errs.push(format!("project not found: {}", proj_rel));
                        continue;
                    }
                };
                let dst = skill_dst_path(project, &skill.name);
                match copy_skill(&skill.src_path, &dst) {
                    Ok(n) => total += n,
                    Err(e) => errs.push(format!("{}→{}: {}", skill.name, proj_rel, e)),
                }
            }
        }
        if errs.is_empty() {
            self.status = format!("✔ push-all: {} files copied across mappings", total);
        } else {
            self.status = format!(
                "push-all: {} files, {} errors (first: {})",
                total,
                errs.len(),
                errs.first().unwrap_or(&String::new())
            );
        }
        Ok(())
    }

    pub fn project_status_for(&self, skill: &Skill, project: &Project) -> SyncStatus {
        let mapped = self.config.is_mapped(&skill.name, &project.relative);
        compute_status(skill, project, mapped)
    }

    pub fn config_path_str(&self) -> String {
        self.config_path.display().to_string()
    }

    pub fn workspace_str(&self) -> String {
        self.workspace_root.display().to_string()
    }

    pub fn skills_root_str(&self) -> String {
        self.skills_root.display().to_string()
    }
}
