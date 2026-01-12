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
use std::{error::Error, io, time::{Duration, Instant, SystemTime, UNIX_EPOCH}};

const DISCORD_APP_ID: &str = "1459887165784723673";

#[derive(PartialEq)]
enum Screen {
    SelectActivity,
    SelectDuration,
    SelectSessions,
    Running,
}

struct App {
    screen: Screen,
    activities: Vec<&'static str>,
    selected_activity: usize,
    duration: u32,
    total_sessions: u32,
    current_session: u32,
    time_remaining: u32,
    is_working: bool,
    is_paused: bool,
    last_tick: Instant,
}

impl App {
    fn new() -> App {
        App {
            screen: Screen::SelectActivity,
            activities: vec![
                "Studying 📚", 
                "Coding 💻", 
                "Deep Work 🧠", 
                "Designing 🎨", 
                "Reading 📖", 
                "Writing ✍️",
                "Exercising 🏋️"
            ],
            selected_activity: 0,
            duration: 25,
            total_sessions: 4,
            current_session: 1,
            time_remaining: 25 * 60,
            is_working: true,
            is_paused: true,
            last_tick: Instant::now(),
        }
    }

    fn calculate_break(&self) -> u32 {
        if self.duration >= 40 { 10 * 60 } else { 5 * 60 }
    }

    fn start_timer(&mut self) {
        self.time_remaining = self.duration * 60;
        self.screen = Screen::Running;
    }

    fn tick(&mut self) {
        if self.screen == Screen::Running && !self.is_paused && self.time_remaining > 0 {
            self.time_remaining -= 1;
        } else if self.time_remaining == 0 {
            if self.is_working {
                if self.current_session >= self.total_sessions {
                    self.screen = Screen::SelectActivity;
                } else {
                    self.is_working = false;
                    self.time_remaining = self.calculate_break();
                }
            } else {
                self.is_working = true;
                self.current_session += 1;
                self.time_remaining = self.duration * 60;
            }
            self.is_paused = true;
        }
    }
}

fn main() -> Result<(), Box<dyn Error>> {
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let mut drpc = DiscordIpcClient::new(DISCORD_APP_ID).ok();
    if let Some(ref mut client) = drpc { let _ = client.connect(); }

    let mut app = App::new();
    let mut list_state = ListState::default();
    list_state.select(Some(0));

    loop {
        terminal.draw(|f| ui(f, &mut app, &mut list_state))?;

        if event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = event::read()? {
                match app.screen {
                    Screen::SelectActivity => match key.code {
                        KeyCode::Up => {
                            app.selected_activity = app.selected_activity.saturating_sub(1);
                            list_state.select(Some(app.selected_activity));
                        }
                        KeyCode::Down => {
                            if app.selected_activity < app.activities.len() - 1 {
                                app.selected_activity += 1;
                                list_state.select(Some(app.selected_activity));
                            }
                        }
                        KeyCode::Enter => app.screen = Screen::SelectDuration,
                        KeyCode::Char('q') => break,
                        _ => {}
                    },
                    Screen::SelectDuration => match key.code {
                        KeyCode::Up => app.duration = (app.duration + 1).min(50),
                        KeyCode::Down => app.duration = (app.duration - 1).max(25),
                        KeyCode::Enter => app.screen = Screen::SelectSessions,
                        _ => {}
                    },
                    Screen::SelectSessions => match key.code {
                        KeyCode::Up => app.total_sessions = (app.total_sessions + 1).min(10),
                        KeyCode::Down => app.total_sessions = (app.total_sessions - 1).max(1),
                        KeyCode::Enter => app.start_timer(),
                        _ => {}
                    },
                    Screen::Running => match key.code {
                        KeyCode::Char(' ') => app.is_paused = !app.is_paused,
                        KeyCode::Char('q') => app.screen = Screen::SelectActivity,
                        _ => {}
                    },
                }
            }
        }

        if app.last_tick.elapsed() >= Duration::from_secs(1) {
            app.tick();
            update_discord(&mut drpc, &app);
            app.last_tick = Instant::now();
        }
    }

    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen, DisableMouseCapture)?;
    Ok(())
}

fn ui(f: &mut ratatui::Frame, app: &mut App, list_state: &mut ListState) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .margin(1)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(10),
            Constraint::Length(3),
        ])
        .split(f.size());

    // 1. Header المحسن
    let header = Paragraph::new("⭐ RUST POMODORO ELITE ⭐")
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD))
        .alignment(Alignment::Center)
        .block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(Color::Indexed(5))));
    f.render_widget(header, chunks[0]);

    // 2. Main Content
    match app.screen {
        Screen::SelectActivity => {
            let items: Vec<ListItem> = app.activities.iter().map(|a| ListItem::new(*a)).collect();
            let list = List::new(items)
                .block(Block::default().title(" [1] Select Your Activity ").borders(Borders::ALL))
                .highlight_style(Style::default().bg(Color::Blue).fg(Color::White).add_modifier(Modifier::BOLD))
                .highlight_symbol("▶ ");
            f.render_stateful_widget(list, chunks[1], list_state);
        }
        Screen::SelectDuration => {
            let p = Paragraph::new(format!("\n\n⏱️ Duration: {} Minutes\n\n☕ Break: {} Minutes", 
                app.duration, if app.duration >= 40 { 10 } else { 5 }))
                .block(Block::default().title(" [2] Set Session Time ").borders(Borders::ALL))
                .alignment(Alignment::Center).style(Style::default().fg(Color::Cyan));
            f.render_widget(p, chunks[1]);
        }
        Screen::SelectSessions => {
            let p = Paragraph::new(format!("\n\n🔄 Total Sessions: {}\n\n🚀 Press Enter to Start!", app.total_sessions))
                .block(Block::default().title(" [3] Set Session Count ").borders(Borders::ALL))
                .alignment(Alignment::Center).style(Style::default().fg(Color::Green));
            f.render_widget(p, chunks[1]);
        }
        Screen::Running => {
            let total_time = if app.is_working { app.duration * 60 } else { app.calculate_break() };
            let progress = (((total_time - app.time_remaining) as f64 / total_time as f64) * 100.0) as u16;
            
            let label = format!("{} / {}", 
                format_time(app.time_remaining), 
                format_time(total_time));

            let color = if app.is_paused { Color::Indexed(8) } 
                        else if app.is_working { Color::Red } 
                        else { Color::Green };
            
            let status_msg = if app.is_paused { "|| PAUSED ||" } 
                            else if app.is_working { "WORKING..." } 
                            else { "BREAK TIME" };

            let gauge = Gauge::default()
                .block(Block::default().title(format!(" {} Session {}/{} ", 
                    status_msg, app.current_session, app.total_sessions)).borders(Borders::ALL))
                .gauge_style(Style::default().fg(color).add_modifier(Modifier::ITALIC))
                .percent(progress.min(100))
                .label(label);
            f.render_widget(gauge, chunks[1]);
        }
    }

    // 3. Footer
    let help_msg = match app.screen {
        Screen::Running => " [Space] Play/Pause | [Q] Back to Menu ",
        _ => " [↑/↓] Navigate | [Enter] Confirm | [Q] Quit ",
    };
    let footer = Paragraph::new(help_msg)
        .alignment(Alignment::Center)
        .style(Style::default().fg(Color::DarkGray))
        .block(Block::default().borders(Borders::ALL));
    f.render_widget(footer, chunks[2]);
}

fn format_time(seconds: u32) -> String {
    format!("{:02}:{:02}", seconds / 60, seconds % 60)
}

fn update_discord(drpc: &mut Option<DiscordIpcClient>, app: &App) {
    if let Some(ref mut client) = drpc {
        let activity_name = app.activities[app.selected_activity];
        
        // 1. الحالة (State)
        let state = if app.screen != Screen::Running {
            "Configuring...".to_string()
        } else if app.is_paused {
            format!("⏸️ Paused: {}", activity_name)
        } else if app.is_working {
            format!("🔥 Focusing: {}", activity_name)
        } else {
            "☕ Taking a Break".to_string()
        };

        // 2. التفاصيل (Details)
        let details = if app.screen == Screen::Running {
            format!("Session {} of {}", app.current_session, app.total_sessions)
        } else {
            "Preparing for success".to_string()
        };

        let mut payload = activity::Activity::new()
            .state(&state)
            .details(&details)
            .assets(activity::Assets::new().large_image("app_icon"));

        // 3. العداد الحي (Live Countdown)
        // يتم إرسال "وقت الانتهاء" لديسكورد وهو يتكفل بالعد التنازلي
        if app.screen == Screen::Running && !app.is_paused {
            let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
            let end_timestamp = now + app.time_remaining as u64;
            payload = payload.timestamps(activity::Timestamps::new().end(end_timestamp as i64));
        }

        let _ = client.set_activity(payload);
    }
}
