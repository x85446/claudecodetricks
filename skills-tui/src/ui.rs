use crate::app::{App, Focus, InputMode, SORT_OPTIONS};
use crate::model::{ProjectKind, SyncStatus};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, List, ListItem, ListState, Paragraph, Wrap},
    Frame,
};

pub fn render(frame: &mut Frame, app: &mut App) {
    let outer = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(5),
            Constraint::Length(3),
            Constraint::Length(2),
        ])
        .split(frame.area());

    render_header(frame, outer[0], app);
    let panes = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(45), Constraint::Percentage(55)])
        .split(outer[1]);
    render_skills(frame, panes[0], app);
    render_projects(frame, panes[1], app);
    render_detail(frame, outer[2], app);
    render_footer(frame, outer[3], app);
    if matches!(app.mode, InputMode::CompanyMenu) {
        render_company_menu(frame, app);
    }
    if matches!(app.mode, InputMode::SortMenu) {
        render_sort_menu(frame, app);
    }
    if matches!(app.mode, InputMode::Help) {
        render_help(frame);
    }
}

fn render_help(frame: &mut Frame) {
    let dim = Style::default().fg(Color::DarkGray);
    let bold = Style::default().add_modifier(Modifier::BOLD);
    let key = Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD);
    let h = Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD);

    let mut lines: Vec<Line> = Vec::new();

    lines.push(Line::from(Span::styled("Status badges (right pane)", h)));
    lines.push(Line::from(vec![
        Span::styled(" [ ] ", Style::default().fg(Color::DarkGray)),
        Span::styled("not mapped", dim),
    ]));
    lines.push(Line::from(vec![
        Span::styled(" [✓] ", Style::default().fg(Color::Red)),
        Span::raw("mapped, not yet synced (destination missing)"),
    ]));
    lines.push(Line::from(vec![
        Span::styled(" [✓] ", Style::default().fg(Color::Green)),
        Span::raw("in sync (file contents identical)"),
    ]));
    lines.push(Line::from(vec![
        Span::styled(" [↑] ", Style::default().fg(Color::Yellow)),
        Span::raw("differs, src mtime newer — push (i)"),
    ]));
    lines.push(Line::from(vec![
        Span::styled(" [↓] ", Style::default().fg(Color::Magenta)),
        Span::raw("differs, dst mtime newer — pull (r)"),
    ]));
    lines.push(Line::from(""));

    lines.push(Line::from(Span::styled("Project kind (right pane)", h)));
    lines.push(Line::from(vec![
        Span::raw("   "),
        Span::raw("git repo (has .git)"),
    ]));
    lines.push(Line::from(vec![
        Span::styled(" * ", Style::default().fg(Color::Cyan)),
        Span::styled(
            "non-git target (has .claude/, e.g. group/umbrella dir)",
            Style::default().fg(Color::Cyan),
        ),
    ]));
    lines.push(Line::from(""));

    lines.push(Line::from(Span::styled("Skill count (left pane)", h)));
    lines.push(Line::from(vec![
        Span::styled(" [N] ", Style::default().fg(Color::Cyan)),
        Span::raw("number of projects mapped to this skill"),
    ]));
    lines.push(Line::from(""));

    let row = |k: &str, desc: &str| -> Line {
        Line::from(vec![
            Span::styled(format!("  {:<12}", k), key),
            Span::raw(desc.to_string()),
        ])
    };

    lines.push(Line::from(Span::styled("Navigation", h)));
    lines.push(row("Tab", "switch focus (skills ↔ projects)"));
    lines.push(row("↑↓ / j k", "move cursor"));
    lines.push(row("PgUp PgDn", "page up/down"));
    lines.push(Line::from(""));

    lines.push(Line::from(Span::styled("Mappings", h)));
    lines.push(row("Space", "toggle mapping for selected skill ↔ project"));
    lines.push(row("i", "push:  source → destination (project mapping)"));
    lines.push(row("G", "push:  source → ~/.claude/skills/<skill> (global)"));
    lines.push(row("r", "pull:  destination → source (confirm)"));
    lines.push(row("a", "push-all: install every mapping (confirm)"));
    lines.push(Line::from(""));

    lines.push(Line::from(Span::styled("Filters", h)));
    lines.push(row("/", "text filter (focused pane) — Esc to clear"));
    lines.push(row("f", "quick filter (company + show all/mapped)"));
    lines.push(Line::from(vec![
        Span::raw("              "),
        Span::styled("inside picker:", bold),
        Span::raw(" "),
        Span::styled("g/i/...", key),
        Span::raw(" company, "),
        Span::styled("*", key),
        Span::raw(" all, "),
        Span::styled("1", key),
        Span::raw(" all proj, "),
        Span::styled("2", key),
        Span::raw(" mapped-only"),
    ]));
    lines.push(row("F", "clear company filter"));
    lines.push(Line::from(""));

    lines.push(Line::from(Span::styled("Sort", h)));
    lines.push(row("s", "open sort picker for focused pane"));
    lines.push(Line::from(vec![
        Span::raw("              "),
        Span::styled("inside picker:", bold),
        Span::raw("  ↑↓ move, Enter apply, "),
        Span::styled("s", key),
        Span::raw(" rapid-fire next+apply"),
    ]));
    lines.push(Line::from(vec![
        Span::raw("              "),
        Span::styled("a/A", key),
        Span::raw(" alpha asc/desc, "),
        Span::styled("g/G", key),
        Span::raw(" age asc/desc, "),
        Span::styled("p/P", key),
        Span::raw(" popularity"),
    ]));
    lines.push(Line::from(""));

    lines.push(Line::from(Span::styled("Other", h)));
    lines.push(row("R", "refresh (rescan skills + projects)"));
    lines.push(row("h / ?", "this help"));
    lines.push(row("q / Esc", "quit"));
    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled(
        "press h, ?, q, or Esc to close",
        dim,
    )));

    let width = 70u16.min(frame.area().width.saturating_sub(4));
    let height = (lines.len() as u16 + 2).min(frame.area().height.saturating_sub(2));
    let area = centered_rect(frame.area(), width, height);
    let block = Block::default()
        .title(" Help ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(Color::Cyan))
        .style(Style::default().bg(Color::Black));
    let p = Paragraph::new(lines).block(block);
    frame.render_widget(Clear, area);
    frame.render_widget(p, area);
}

fn render_sort_menu(frame: &mut Frame, app: &App) {
    let pane = match app.focus {
        Focus::Skills => "Skills",
        Focus::Projects => "Projects",
    };
    let current = match app.focus {
        Focus::Skills => app.skill_sort,
        Focus::Projects => app.project_sort,
    };
    let mut lines: Vec<Line> = Vec::new();
    lines.push(Line::from(vec![
        Span::styled("Sort: ", Style::default().fg(Color::DarkGray)),
        Span::styled(pane, Style::default().add_modifier(Modifier::BOLD)),
        Span::styled(
            format!("  ({})", current.long_label()),
            Style::default().fg(Color::DarkGray),
        ),
    ]));
    lines.push(Line::from(""));
    for (i, (key, order)) in SORT_OPTIONS.iter().enumerate() {
        let active = *order == current;
        let cursor = i == app.sort_menu_cursor;
        let arrow = if cursor { "▶ " } else { "  " };
        let arrow_style = Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD);
        let label_style = if active {
            Style::default()
                .fg(Color::Green)
                .add_modifier(Modifier::BOLD)
        } else {
            Style::default()
        };
        lines.push(Line::from(vec![
            Span::styled(arrow, arrow_style),
            Span::raw("("),
            Span::styled(
                key.to_string(),
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(") "),
            Span::styled(order.long_label(), label_style),
        ]));
    }
    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled(
        "↑↓ move   Enter apply   s next+apply   Esc close",
        Style::default().fg(Color::DarkGray),
    )));

    let height = (lines.len() as u16 + 2).min(frame.area().height.saturating_sub(2));
    let width = 50u16.min(frame.area().width.saturating_sub(4));
    let area = centered_rect(frame.area(), width, height);
    let block = Block::default()
        .title(" Sort options ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(Color::Yellow))
        .style(Style::default().bg(Color::Black));
    let p = Paragraph::new(lines).block(block);
    frame.render_widget(Clear, area);
    frame.render_widget(p, area);
}

fn render_header(frame: &mut Frame, area: Rect, app: &App) {
    let title = Line::from(vec![
        Span::styled(
            " skills-tui ",
            Style::default().fg(Color::Black).bg(Color::Cyan).add_modifier(Modifier::BOLD),
        ),
        Span::raw(" "),
        Span::styled(
            format!("workspace={}", app.workspace_str()),
            Style::default().fg(Color::DarkGray),
        ),
        Span::raw("  "),
        Span::styled(
            format!("config={}", app.config_path_str()),
            Style::default().fg(Color::DarkGray),
        ),
    ]);
    let block = Block::default().borders(Borders::BOTTOM).border_style(Style::default().fg(Color::DarkGray));
    let p = Paragraph::new(title).block(block);
    frame.render_widget(p, area);
}

fn render_skills(frame: &mut Frame, area: Rect, app: &mut App) {
    let visible = app.filtered_skills();
    let items: Vec<ListItem> = visible
        .iter()
        .map(|i| {
            let s = &app.skills[*i];
            let mapped_count = app.config.projects_for(&s.name).len();
            let badge = if mapped_count > 0 {
                format!(" [{}] ", mapped_count)
            } else {
                "  ─  ".to_string()
            };
            let age = s.relative_age();
            let line = Line::from(vec![
                Span::styled(badge, Style::default().fg(Color::Cyan)),
                Span::styled(
                    format!("{:>5} ", age),
                    Style::default().fg(Color::DarkGray),
                ),
                Span::styled(s.name.clone(), Style::default().add_modifier(Modifier::BOLD)),
                Span::raw("  "),
                Span::styled(
                    truncate(&s.description, 60),
                    Style::default().fg(Color::DarkGray),
                ),
            ]);
            ListItem::new(line)
        })
        .collect();

    let sort = app.skill_sort.label();
    let editing = matches!(app.mode, InputMode::Filter) && app.focus == Focus::Skills;
    let title = if editing {
        format!(" Skills [{}] (filter: {}_) ", sort, app.filter)
    } else if !app.filter.is_empty() {
        format!(
            " Skills [{}] ({}/{}) [filter: {}] ",
            sort,
            visible.len(),
            app.skills.len(),
            app.filter
        )
    } else {
        format!(" Skills [{}] ({}) ", sort, app.skills.len())
    };

    let border_style = if app.focus == Focus::Skills {
        Style::default().fg(Color::Cyan)
    } else {
        Style::default().fg(Color::DarkGray)
    };

    let list = List::new(items)
        .block(
            Block::default()
                .title(title)
                .borders(Borders::ALL)
                .border_style(border_style),
        )
        .highlight_style(
            Style::default()
                .bg(Color::Blue)
                .fg(Color::White)
                .add_modifier(Modifier::BOLD),
        )
        .highlight_symbol("▶ ");

    let mut state = ListState::default();
    if !visible.is_empty() {
        state.select(Some(app.skill_idx.min(visible.len().saturating_sub(1))));
    }
    frame.render_stateful_widget(list, area, &mut state);
}

fn render_projects(frame: &mut Frame, area: Rect, app: &mut App) {
    let skill_opt = app.current_skill().cloned();
    let visible = app.visible_projects();
    let items: Vec<ListItem> = visible
        .iter()
        .map(|i| {
            let p = &app.projects[*i];
            let status = match &skill_opt {
                Some(s) => app.project_status_for(s, p),
                None => SyncStatus::NotMapped,
            };
            let badge_style = match status {
                SyncStatus::InSync => Style::default().fg(Color::Green),
                SyncStatus::SrcNewer => Style::default().fg(Color::Yellow),
                SyncStatus::DstNewer => Style::default().fg(Color::Magenta),
                SyncStatus::NotInstalled => Style::default().fg(Color::Red),
                SyncStatus::NotMapped => Style::default().fg(Color::DarkGray),
            };
            let name_style = match (status, p.kind) {
                (SyncStatus::NotMapped, ProjectKind::Git) => Style::default().fg(Color::DarkGray),
                (SyncStatus::NotMapped, ProjectKind::Claude) => Style::default().fg(Color::DarkGray),
                (_, ProjectKind::Claude) => Style::default().fg(Color::Cyan),
                (_, ProjectKind::Git) => Style::default(),
            };
            let kind_mark = match p.kind {
                ProjectKind::Git => " ",
                ProjectKind::Claude => "*",
            };
            let kind_style = match p.kind {
                ProjectKind::Git => Style::default().fg(Color::DarkGray),
                ProjectKind::Claude => Style::default().fg(Color::Cyan),
            };
            let line = Line::from(vec![
                Span::styled(status.badge().to_string(), badge_style),
                Span::styled(
                    format!("{:>5} ", p.relative_age()),
                    Style::default().fg(Color::DarkGray),
                ),
                Span::styled(kind_mark, kind_style),
                Span::raw(" "),
                Span::styled(p.relative.clone(), name_style),
            ]);
            ListItem::new(line)
        })
        .collect();

    let psort = app.project_sort.label();
    let mapped_tag = if app.mapped_only { " mapped-only" } else { "" };
    let editing = matches!(app.mode, InputMode::Filter) && app.focus == Focus::Projects;
    let filter_tag = if editing {
        format!(" filter: {}_", app.project_filter)
    } else if !app.project_filter.is_empty() {
        format!(" filter: {}", app.project_filter)
    } else {
        String::new()
    };
    let skill_tag = match &skill_opt {
        Some(s) => format!(" → {}", s.name),
        None => String::new(),
    };
    let company_tag = match &app.company_filter {
        Some(c) => format!(" [{}]", c),
        None => String::new(),
    };
    let header = format!(
        " Projects [{}{}{}]{}{} ({}/{}) ",
        psort,
        mapped_tag,
        filter_tag,
        skill_tag,
        company_tag,
        visible.len(),
        app.projects.len()
    );

    let border_style = if app.focus == Focus::Projects {
        Style::default().fg(Color::Cyan)
    } else {
        Style::default().fg(Color::DarkGray)
    };

    let list = List::new(items)
        .block(
            Block::default()
                .title(header)
                .borders(Borders::ALL)
                .border_style(border_style),
        )
        .highlight_style(
            Style::default()
                .bg(Color::Blue)
                .fg(Color::White)
                .add_modifier(Modifier::BOLD),
        )
        .highlight_symbol("▶ ");

    let mut state = ListState::default();
    if !visible.is_empty() {
        state.select(Some(app.project_idx.min(visible.len().saturating_sub(1))));
    }
    frame.render_stateful_widget(list, area, &mut state);
}

fn render_company_menu(frame: &mut Frame, app: &App) {
    let dim = Style::default().fg(Color::DarkGray);
    let key_style = Style::default()
        .fg(Color::Yellow)
        .add_modifier(Modifier::BOLD);
    let active_dot = Style::default().fg(Color::Green);
    let header = Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD);

    let companies = app.companies();
    let mut lines: Vec<Line> = Vec::new();

    lines.push(Line::from(Span::styled("Company:", header)));
    for (name, ch) in &companies {
        let mut spans: Vec<Span> = Vec::new();
        let mut highlighted = false;
        for c in name.chars() {
            if !highlighted && c.to_ascii_lowercase() == *ch {
                spans.push(Span::raw("("));
                spans.push(Span::styled(c.to_string(), key_style));
                spans.push(Span::raw(")"));
                highlighted = true;
            } else {
                spans.push(Span::raw(c.to_string()));
            }
        }
        let active = app.company_filter.as_deref() == Some(name.as_str());
        if active {
            spans.insert(0, Span::styled("● ", active_dot));
        } else {
            spans.insert(0, Span::raw("  "));
        }
        lines.push(Line::from(spans));
    }
    let star_active = app.company_filter.is_none();
    let star_prefix = if star_active { "● " } else { "  " };
    lines.push(Line::from(vec![
        Span::styled(
            star_prefix,
            if star_active {
                active_dot
            } else {
                Style::default()
            },
        ),
        Span::raw("("),
        Span::styled("*", key_style),
        Span::raw(") all companies"),
    ]));

    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled("Show:", header)));

    let all_active = !app.mapped_only;
    lines.push(Line::from(vec![
        Span::styled(
            if all_active { "● " } else { "  " },
            if all_active {
                active_dot
            } else {
                Style::default()
            },
        ),
        Span::raw("("),
        Span::styled("1", key_style),
        Span::raw(") all projects"),
    ]));

    let mapped_active = app.mapped_only;
    lines.push(Line::from(vec![
        Span::styled(
            if mapped_active { "● " } else { "  " },
            if mapped_active {
                active_dot
            } else {
                Style::default()
            },
        ),
        Span::raw("("),
        Span::styled("2", key_style),
        Span::raw(") only projects with any skill mapped"),
    ]));

    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled("Esc cancel", dim)));

    let height = (lines.len() as u16 + 2).min(frame.area().height.saturating_sub(2));
    let width = 50u16.min(frame.area().width.saturating_sub(4));
    let area = centered_rect(frame.area(), width, height);
    let block = Block::default()
        .title(" Quick Filter ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(Color::Yellow))
        .style(Style::default().bg(Color::Black));
    let p = Paragraph::new(lines).block(block);
    frame.render_widget(Clear, area);
    frame.render_widget(p, area);
}

fn centered_rect(area: Rect, width: u16, height: u16) -> Rect {
    let x = area.x + area.width.saturating_sub(width) / 2;
    let y = area.y + area.height.saturating_sub(height) / 2;
    Rect {
        x,
        y,
        width: width.min(area.width),
        height: height.min(area.height),
    }
}

fn render_detail(frame: &mut Frame, area: Rect, app: &App) {
    let mut lines: Vec<Line> = Vec::new();
    if let Some(s) = app.current_skill() {
        lines.push(Line::from(vec![
            Span::styled("skill: ", Style::default().fg(Color::DarkGray)),
            Span::styled(s.name.clone(), Style::default().add_modifier(Modifier::BOLD)),
            Span::raw("    "),
            Span::styled("src: ", Style::default().fg(Color::DarkGray)),
            Span::raw(s.src_path.display().to_string()),
        ]));
        if !s.description.is_empty() {
            lines.push(Line::from(Span::styled(
                truncate(&s.description, 200),
                Style::default().fg(Color::Gray),
            )));
        }
    } else {
        lines.push(Line::from("(no skill selected)"));
    }

    let p = Paragraph::new(lines)
        .block(
            Block::default()
                .title(" Detail ")
                .borders(Borders::ALL)
                .border_style(Style::default().fg(Color::DarkGray)),
        )
        .wrap(Wrap { trim: true });
    frame.render_widget(p, area);
}

fn render_footer(frame: &mut Frame, area: Rect, app: &App) {
    let help_line = match app.mode {
        InputMode::CompanyMenu => Line::from(vec![
            Span::styled(" QUICK FILTER ", Style::default().fg(Color::Black).bg(Color::Yellow)),
            Span::raw("  press the highlighted letter, "),
            Span::styled("*", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" for all, "),
            Span::styled("Esc", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" cancel"),
        ]),
        InputMode::Help => Line::from(vec![
            Span::styled(" HELP ", Style::default().fg(Color::Black).bg(Color::Cyan)),
            Span::raw("  press "),
            Span::styled("h", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(", "),
            Span::styled("?", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(", "),
            Span::styled("q", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(", or "),
            Span::styled("Esc", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" to close"),
        ]),
        InputMode::SortMenu => Line::from(vec![
            Span::styled(" SORT ", Style::default().fg(Color::Black).bg(Color::Yellow)),
            Span::raw("  ↑↓ move, Enter apply, "),
            Span::styled("s", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" next+apply, "),
            Span::styled("Esc", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" close"),
        ]),
        InputMode::Filter => Line::from(vec![
            Span::styled(" FILTER ", Style::default().fg(Color::Black).bg(Color::Yellow)),
            Span::raw("  type to filter   "),
            Span::styled("Enter", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" apply   "),
            Span::styled("Esc", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" cancel"),
        ]),
        InputMode::Confirm(_) => Line::from(vec![
            Span::styled(" CONFIRM ", Style::default().fg(Color::Black).bg(Color::Red)),
            Span::raw("  press "),
            Span::styled("y", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" to proceed, "),
            Span::styled("n/Esc", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" to cancel"),
        ]),
        InputMode::Normal => Line::from(vec![
            Span::styled("Tab", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" focus  "),
            Span::styled("↑↓/jk", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" move  "),
            Span::styled("Space", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" toggle map  "),
            Span::styled("i", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" push  "),
            Span::styled("G", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" global  "),
            Span::styled("r", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" pull  "),
            Span::styled("a", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" push-all  "),
            Span::styled("f", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" company  "),
            Span::styled("s", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" sort  "),
            Span::styled("/", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" filter  "),
            Span::styled("R", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" refresh  "),
            Span::styled("h", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" help  "),
            Span::styled("q", Style::default().add_modifier(Modifier::BOLD)),
            Span::raw(" quit"),
        ]),
    };

    let status_line = Line::from(Span::styled(
        format!(" {} ", app.status),
        Style::default().fg(Color::Green),
    ));

    let stack = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(1), Constraint::Length(1)])
        .split(area);
    frame.render_widget(Paragraph::new(help_line), stack[0]);
    frame.render_widget(Paragraph::new(status_line), stack[1]);
}

fn truncate(s: &str, max: usize) -> String {
    if s.chars().count() <= max {
        s.to_string()
    } else {
        let mut out: String = s.chars().take(max.saturating_sub(1)).collect();
        out.push('…');
        out
    }
}
