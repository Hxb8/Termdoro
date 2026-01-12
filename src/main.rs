use std::{error::Error, io, time::{Duration, Instant, SystemTime, UNIX_EPOCH}};
use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use discord_rich_presence::{activity, DiscordIpc, DiscordIpcClient};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Alignment, Constraint, Direction, Layout},
    style::{Color, Modifier, Style},
    widgets::{Block, Borders, Gauge, List, ListItem, ListState, Paragraph},
    Terminal,
};

const APP_ID: &str = "1459887165784723673";

#[derive(PartialEq)]
enum Screen {
    Menu,
    SetDuration,
    SetSessions,
    Timer,
}

struct App {
    screen: Screen,
    acts: Vec<&'static str>,
    idx: usize,
    mins: u32,
    total: u32,
    current: u32,
    rem: u32,
    work: bool,
    paused: bool,
    tick: Instant,
}

impl App {
    fn new() -> Self {
        Self {
            screen: Screen::Menu,
            acts: vec!["Studying 📚", "Coding 💻", "Deep Work 🧠", "Designing 🎨", "Reading 📖", "Writing ✍️"],
            idx: 0,
            mins: 25,
            total: 4,
            current: 1,
            rem: 25 * 60,
            work: true,
            paused: true,
            tick: Instant::now(),
        }
    }

    fn on_tick(&mut self) {
        if self.screen != Screen::Timer || self.paused || self.rem == 0 { return; }
        self.rem -= 1;
        
        if self.rem == 0 {
            if self.work {
                if self.current >= self.total {
                    self.screen = Screen::Menu;
                } else {
                    self.work = false;
                    self.rem = if self.mins >= 40 { 10 * 60 } else { 5 * 60 };
                }
            } else {
                self.work = true;
                self.current += 1;
                self.rem = self.mins * 60;
            }
            self.paused = true;
        }
    }
}

fn main() -> Result<(), Box<dyn Error>> {
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)?;
    
    let mut terminal = Terminal::new(CrosstermBackend::new(stdout))?;
    let mut drpc = DiscordIpcClient::new(APP_ID).ok();
    if let Some(ref mut c) = drpc { let _ = c.connect(); }

    let mut app = App::new();
    let mut l_state = ListState::default();
    l_state.select(Some(0));

    loop {
        terminal.draw(|f| draw_ui(f, &mut app, &mut l_state))?;

        if event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = event::read()? {
                match app.screen {
                    Screen::Menu => match key.code {
                        KeyCode::Up => { app.idx = app.idx.saturating_sub(1); l_state.select(Some(app.idx)); }
                        KeyCode::Down => { if app.idx < app.acts.len() - 1 { app.idx += 1; l_state.select(Some(app.idx)); } }
                        KeyCode::Enter => app.screen = Screen::SetDuration,
                        KeyCode::Char('q') => break,
                        _ => {}
                    },
                    Screen::SetDuration => match key.code {
                        KeyCode::Up => app.mins = (app.mins + 1).min(60),
                        KeyCode::Down => app.mins = (app.mins - 1).max(1),
                        KeyCode::Enter => app.screen = Screen::SetSessions,
                        _ => {}
                    },
                    Screen::SetSessions => match key.code {
                        KeyCode::Up => app.total = (app.total + 1).min(12),
                        KeyCode::Down => app.total = (app.total - 1).max(1),
                        KeyCode::Enter => { app.rem = app.mins * 60; app.screen = Screen::Timer; app.paused = false; }
                        _ => {}
                    },
                    Screen::Timer => match key.code {
                        KeyCode::Char(' ') => app.paused = !app.paused,
                        KeyCode::Char('q') => app.screen = Screen::Menu,
                        _ => {}
                    },
                }
            }
        }

        if app.tick.elapsed() >= Duration::from_secs(1) {
            app.on_tick();
            update_presence(&mut drpc, &app);
            app.tick = Instant::now();
        }
    }

    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen, DisableMouseCapture)?;
    Ok(())
}

fn draw_ui(f: &mut ratatui::Frame, app: &mut App, l_state: &mut ListState) {
    let rc = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(3), Constraint::Min(10), Constraint::Length(3)])
        .split(f.size());

    f.render_widget(
        Paragraph::new("🦀 Rust Pomo Elite").alignment(Alignment::Center)
            .block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(Color::Magenta))),
        rc[0]
    );

    match app.screen {
        Screen::Menu => {
            let items: Vec<ListItem> = app.acts.iter().map(|i| ListItem::new(*i)).collect();
            f.render_stateful_widget(
                List::new(items).block(Block::default().title(" Select Activity ").borders(Borders::ALL))
                    .highlight_style(Style::default().bg(Color::Magenta).add_modifier(Modifier::BOLD)),
                rc[1], l_state
            );
        }
        Screen::SetDuration => {
            f.render_widget(
                Paragraph::new(format!("\nFocus Time: {} min\nBreak: {} min", app.mins, if app.mins >= 40 {10} else {5}))
                    .alignment(Alignment::Center).block(Block::default().title(" Duration ").borders(Borders::ALL)),
                rc[1]
            );
        }
        Screen::SetSessions => {
            f.render_widget(
                Paragraph::new(format!("\nTotal Sessions: {}\n\nPress Enter to Start", app.total))
                    .alignment(Alignment::Center).block(Block::default().title(" Sessions ").borders(Borders::ALL)),
                rc[1]
            );
        }
        Screen::Timer => {
            let total = if app.work { app.mins * 60 } else { if app.mins >= 40 { 600 } else { 300 } };
            let pct = ((total - app.rem) as f64 / total as f64 * 100.0) as u16;
            let color = if app.paused { Color::DarkGray } else if app.work { Color::Red } else { Color::Green };
            
            f.render_widget(
                Gauge::default().block(Block::default().title(format!(" Session {}/{} ", app.current, app.total)).borders(Borders::ALL))
                    .gauge_style(Style::default().fg(color))
                    .percent(pct).label(format!("{}:{:02}", app.rem / 60, app.rem % 60)),
                rc[1]
            );
        }
    }

    f.render_widget(
        Paragraph::new(" [Space] Pause | [Q] Menu | [Esc] Quit ").alignment(Alignment::Center),
        rc[2]
    );
}

fn update_presence(drpc: &mut Option<DiscordIpcClient>, app: &App) {
    let client = match drpc { Some(c) => c, None => return };

    let (state, details) = match app.screen {
        Screen::Timer => {
            let s = if app.paused { format!("⏸️ Paused: {}", app.acts[app.idx]) } 
                    else if app.work { format!("🔥 Focusing: {}", app.acts[app.idx]) }
                    else { "☕ Break Time".into() };
            (s, format!("Session {}/{}", app.current, app.total))
        },
        _ => ("Configuring...".into(), "Main Menu".into()),
    };

    let mut payload = activity::Activity::new()
        .state(&state)
        .details(&details)
        .assets(activity::Assets::new().large_image("app_icon"));

    if app.screen == Screen::Timer && !app.paused {
        let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
        payload = payload.timestamps(activity::Timestamps::new().end((now + app.rem as u64) as i64));
    }

    let _ = client.set_activity(payload);
}
