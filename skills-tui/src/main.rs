mod app;
mod model;
mod ui;

use anyhow::{Context, Result};
use app::{App, ConfirmAction, Focus, InputMode};
use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode, KeyEventKind, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::{backend::CrosstermBackend, Terminal};
use std::io;
use std::time::Duration;

fn main() -> Result<()> {
    let home = dirs::home_dir().context("could not resolve home dir")?;
    let workspace_root = home.join("workspace");
    let skills_root = workspace_root
        .join("x85446")
        .join("claudecodetricks")
        .join("skills");
    let config_path = skills_root.join("skill-mappings.toml");
    let global_skills_root = home.join(".claude").join("skills");

    if !skills_root.exists() {
        anyhow::bail!("skills root does not exist: {}", skills_root.display());
    }
    if !workspace_root.exists() {
        anyhow::bail!("workspace root does not exist: {}", workspace_root.display());
    }

    let args: Vec<String> = std::env::args().collect();
    if args.iter().any(|a| a == "--list" || a == "-l") {
        let app = App::new(workspace_root, skills_root, config_path, global_skills_root.clone())?;
        println!("workspace: {}", app.workspace_str());
        println!("skills:    {}", app.skills_root_str());
        println!("config:    {}", app.config_path_str());
        println!();
        println!("=== {} skills (most-recent first) ===", app.skills.len());
        for s in &app.skills {
            let mapped = app.config.projects_for(&s.name).len();
            println!(
                "  [{:>2}] {:>5}  {:<28} {}",
                mapped,
                s.relative_age(),
                s.name,
                truncate(&s.description, 60)
            );
        }
        println!();
        println!("=== {} projects (most-recent first) ===", app.projects.len());
        for p in &app.projects {
            println!("  {:>5}  {}", p.relative_age(), p.relative);
        }
        return Ok(());
    }

    let mut app = App::new(workspace_root, skills_root, config_path, global_skills_root.clone())?;

    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture).context("enter alt screen")?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend).context("create terminal")?;

    let res = run_loop(&mut terminal, &mut app);

    disable_raw_mode().ok();
    execute!(terminal.backend_mut(), LeaveAlternateScreen, DisableMouseCapture).ok();
    terminal.show_cursor().ok();

    res
}

fn run_loop<B: ratatui::backend::Backend>(
    terminal: &mut Terminal<B>,
    app: &mut App,
) -> Result<()> {
    loop {
        app.tick();
        terminal.draw(|f| ui::render(f, app))?;
        if app.should_quit {
            break;
        }
        if event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = event::read()? {
                if key.kind != KeyEventKind::Press {
                    continue;
                }
                handle_key(app, key.code, key.modifiers)?;
            }
        }
    }
    Ok(())
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

fn handle_key(app: &mut App, code: KeyCode, mods: KeyModifiers) -> Result<()> {
    match &app.mode {
        InputMode::Filter => match code {
            KeyCode::Esc => {
                app.mode = InputMode::Normal;
                match app.focus {
                    Focus::Skills => {
                        app.filter.clear();
                        app.skill_idx = 0;
                    }
                    Focus::Projects => {
                        app.project_filter.clear();
                        app.project_idx = 0;
                    }
                }
            }
            KeyCode::Enter => {
                app.mode = InputMode::Normal;
            }
            KeyCode::Backspace => match app.focus {
                Focus::Skills => {
                    app.filter.pop();
                    app.skill_idx = 0;
                }
                Focus::Projects => {
                    app.project_filter.pop();
                    app.project_idx = 0;
                }
            },
            KeyCode::Char(c) => match app.focus {
                Focus::Skills => {
                    app.filter.push(c);
                    app.skill_idx = 0;
                }
                Focus::Projects => {
                    app.project_filter.push(c);
                    app.project_idx = 0;
                }
            },
            _ => {}
        },
        InputMode::Help => match code {
            KeyCode::Esc | KeyCode::Char('q') | KeyCode::Char('h') | KeyCode::Char('?') => {
                app.mode = InputMode::Normal;
            }
            _ => {}
        },
        InputMode::SortMenu => match code {
            KeyCode::Esc => app.close_sort_menu(),
            KeyCode::Up | KeyCode::Char('k') => app.sort_menu_move(-1),
            KeyCode::Down | KeyCode::Char('j') => app.sort_menu_move(1),
            KeyCode::Enter => app.sort_menu_apply_cursor(),
            KeyCode::Char('s') => app.sort_menu_advance(),
            KeyCode::Char(c) => {
                let _ = app.sort_menu_jump(c);
            }
            _ => {}
        },
        InputMode::CompanyMenu => {
            match code {
                KeyCode::Esc => {
                    app.mode = InputMode::Normal;
                }
                KeyCode::Char('*') | KeyCode::Char('0') => {
                    app.set_company_filter(None);
                    app.mode = InputMode::Normal;
                    app.status = "company filter cleared".to_string();
                }
                KeyCode::Char('1') => {
                    app.set_mapped_only(false);
                    app.mode = InputMode::Normal;
                }
                KeyCode::Char('2') => {
                    app.set_mapped_only(true);
                    app.mode = InputMode::Normal;
                }
                KeyCode::Char(c) => {
                    let lc = c.to_ascii_lowercase();
                    let companies = app.companies();
                    if let Some((name, _)) = companies.iter().find(|(_, ch)| *ch == lc) {
                        let n = name.clone();
                        app.set_company_filter(Some(n.clone()));
                        app.mode = InputMode::Normal;
                        app.status = format!("company: {}", n);
                    }
                }
                _ => {}
            }
        }
        InputMode::Confirm(action) => {
            let action = action.clone();
            match code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    match action {
                        ConfirmAction::PushAll => app.push_all()?,
                        ConfirmAction::PullSelected => app.pull_selected()?,
                    }
                    app.mode = InputMode::Normal;
                }
                KeyCode::Esc | KeyCode::Char('n') | KeyCode::Char('N') => {
                    app.mode = InputMode::Normal;
                    app.status = "(cancelled)".to_string();
                }
                _ => {}
            }
        }
        InputMode::Normal => match code {
            KeyCode::Char('q') | KeyCode::Esc => app.should_quit = true,
            KeyCode::Char('c') if mods.contains(KeyModifiers::CONTROL) => app.should_quit = true,
            KeyCode::Tab => {
                app.focus = match app.focus {
                    Focus::Skills => Focus::Projects,
                    Focus::Projects => Focus::Skills,
                };
            }
            KeyCode::BackTab => {
                app.focus = match app.focus {
                    Focus::Skills => Focus::Projects,
                    Focus::Projects => Focus::Skills,
                };
            }
            KeyCode::Up | KeyCode::Char('k') => match app.focus {
                Focus::Skills => app.move_skill(-1),
                Focus::Projects => app.move_project(-1),
            },
            KeyCode::Down | KeyCode::Char('j') => match app.focus {
                Focus::Skills => app.move_skill(1),
                Focus::Projects => app.move_project(1),
            },
            KeyCode::PageUp => match app.focus {
                Focus::Skills => app.move_skill(-10),
                Focus::Projects => app.move_project(-10),
            },
            KeyCode::PageDown => match app.focus {
                Focus::Skills => app.move_skill(10),
                Focus::Projects => app.move_project(10),
            },
            KeyCode::Char(' ') => app.toggle_mapping()?,
            KeyCode::Char('i') => app.push_selected()?,
            KeyCode::Char('G') => app.push_global()?,
            KeyCode::Char('r') => {
                app.mode = InputMode::Confirm(ConfirmAction::PullSelected);
                app.status = "Pull dst → src will OVERWRITE the source skill files. Confirm? (y/n)".to_string();
            }
            KeyCode::Char('a') => {
                app.mode = InputMode::Confirm(ConfirmAction::PushAll);
                app.status = "Push all mapped skills to all destinations? (y/n)".to_string();
            }
            KeyCode::Char('R') => app.refresh()?,
            KeyCode::Char('h') | KeyCode::Char('?') => {
                app.mode = InputMode::Help;
            }
            KeyCode::Char('s') => app.open_sort_menu(),
            KeyCode::Char('f') => {
                app.mode = InputMode::CompanyMenu;
            }
            KeyCode::Char('F') => {
                app.set_company_filter(None);
                app.status = "company filter cleared".to_string();
            }
            KeyCode::Char('/') => {
                app.mode = InputMode::Filter;
                match app.focus {
                    Focus::Skills => app.filter.clear(),
                    Focus::Projects => app.project_filter.clear(),
                }
            }
            _ => {}
        },
    }
    Ok(())
}
