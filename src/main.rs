use std::{error::Error, io, fs, process::{Command, Stdio}, sync::{Arc, Mutex}, time::{Duration, Instant, SystemTime, UNIX_EPOCH}};
use crossterm::{event::{self, DisableMouseCapture, Event, KeyCode}, execute, terminal::*};
use discord_rich_presence::{activity, DiscordIpc, DiscordIpcClient};
use ratatui::{backend::CrosstermBackend, layout::*, style::*, widgets::*, Terminal, text::{Line, Span}};
use rodio::{Decoder, OutputStream, Sink, Source};

const APP_ID: &str = "1459887165784723673";

#[derive(Debug, PartialEq, Clone, Copy)]
enum Theme { Cyan, Magenta, Green, Yellow, Red }

impl Theme {
    fn color(&self) -> Color {
        match self {
            Theme::Cyan => Color::Cyan, Theme::Magenta => Color::Magenta,
            Theme::Green => Color::Green, Theme::Yellow => Color::Yellow, Theme::Red => Color::Red,
        }
    }
}

#[derive(PartialEq, Clone)]
enum Screen { Activity, Duration, Sessions, BGM, BGMImport, Settings, Timer }

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
    input: String,
    status_msg: Arc<Mutex<String>>,
    is_downloading: Arc<Mutex<bool>>,
    download_done: Arc<Mutex<bool>>,
    bgm_list: Vec<String>,
    bgm_idx: usize,
    sink: Option<Sink>,
    _stream: Option<OutputStream>,
    notifications_enabled: bool,
    theme: Theme,
    settings_cursor: usize,
    volume: f32,
}

impl App {
    fn new() -> Self {
        let mut app = Self {
            screen: Screen::Activity,
            acts: vec!["Studying 📚", "Coding 💻", "Deep Work 🧠", "Reading 📖"],
            idx: 0, mins: 25, total: 4, current: 1, rem: 25 * 60,
            work: true, paused: true, tick: Instant::now(),
            input: String::new(),
            status_msg: Arc::new(Mutex::new("Ready".into())),
            is_downloading: Arc::new(Mutex::new(false)),
            download_done: Arc::new(Mutex::new(false)),
            bgm_list: vec!["None".into()], bgm_idx: 0, sink: None, _stream: None,
            notifications_enabled: true,
            theme: Theme::Cyan,
            settings_cursor: 0,
            volume: 0.5,
        };
        app.refresh_bgm();
        app
    }

    fn refresh_bgm(&mut self) {
        let mut list = vec!["None".into()];
        let _ = fs::create_dir_all("./bgm");
        if let Ok(entries) = fs::read_dir("./bgm") {
            for entry in entries.flatten() {
                let name = entry.file_name().to_string_lossy().into_owned();
                if name.ends_with(".mp3") { list.push(name); }
            }
        }
        self.bgm_list = list;
    }

    fn start_download(&mut self) {
        let url = self.input.clone();
        let status = self.status_msg.clone();
        let downloading = self.is_downloading.clone();
        let done = self.download_done.clone();
        self.input.clear();

        std::thread::spawn(move || {
            if let Ok(mut d) = downloading.lock() { *d = true; }
            if let Ok(mut s) = status.lock() { *s = "📥 Downloading background song...".into(); }
            
            let cmd = Command::new("yt-dlp")
                .args(["-x", "--audio-format", "mp3", "--quiet", "--no-warnings", "-o", "bgm/%(title)s.%(ext)s", &url])
                .stdout(Stdio::null()).stderr(Stdio::null()).status();

            let msg = if let Ok(s) = cmd { if s.success() { "✅ Success! Press ENTER" } else { "❌ Failed" } } else { "❌ yt-dlp missing" };
            if let Ok(mut s) = status.lock() { *s = msg.into(); }
            if let Ok(mut dn) = done.lock() { *dn = true; }
        });
    }

    fn play_bgm(&mut self) {
        self.stop_bgm();
        if self.bgm_idx == 0 { return; }
        let path = format!("./bgm/{}", self.bgm_list[self.bgm_idx]);
        if let Ok(file) = fs::File::open(path) {
            if let Ok((stream, handle)) = OutputStream::try_default() {
                if let Ok(sink) = Sink::try_new(&handle) {
                    if let Ok(source) = Decoder::new(io::BufReader::new(file)) {
                        sink.set_volume(self.volume);
                        sink.append(source.convert_samples::<f32>().repeat_infinite());
                        if self.paused { sink.pause(); }
                        self.sink = Some(sink);
                        self._stream = Some(stream);
                    }
                }
            }
        }
    }

    fn stop_bgm(&mut self) {
        if let Some(s) = &self.sink { s.stop(); }
        self.sink = None; self._stream = None;
    }

    fn adjust_volume(&mut self, delta: f32) {
        self.volume = (self.volume + delta).clamp(0.0, 1.0);
        if let Some(s) = &self.sink { s.set_volume(self.volume); }
    }

    fn toggle_pause(&mut self) {
        self.paused = !self.paused;
        if let Some(s) = &self.sink {
            if self.paused { s.pause(); } else { s.play(); }
        }
    }

    fn on_tick(&mut self) {
        if self.screen != Screen::Timer || self.paused || self.rem == 0 { return; }
        self.rem -= 1;
        if self.rem == 0 {
            let (t, b);
            if self.work {
                if self.current >= self.total { t = "Done! 🎉"; b = "All sessions finished!"; self.screen = Screen::Activity; self.stop_bgm(); }
                else { self.work = false; self.rem = if self.mins >= 40 { 600 } else { 300 }; t = "Break! ☕"; b = "Time to rest."; }
            } else { self.work = true; self.current += 1; self.rem = self.mins * 60; t = "Work! 🔥"; b = "Focus time."; }
            if self.notifications_enabled { let _ = notify_rust::Notification::new().summary(t).body(b).show(); }
            self.paused = true;
            if let Some(s) = &self.sink { s.pause(); }
        }
    }
}

fn main() -> Result<(), Box<dyn Error>> {
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen)?;
    let mut terminal = Terminal::new(CrosstermBackend::new(stdout))?;
    let mut drpc = DiscordIpcClient::new(APP_ID).ok();
    if let Some(ref mut c) = drpc { let _ = c.connect(); }

    let mut app = App::new();
    let mut l_state = ListState::default(); l_state.select(Some(0));

    loop {
        terminal.draw(|f| ui(f, &mut app, &mut l_state))?;
        if event::poll(Duration::from_millis(50))? {
            if let Event::Key(key) = event::read()? {
                if *app.is_downloading.lock().unwrap() {
                    if *app.download_done.lock().unwrap() && key.code == KeyCode::Enter {
                        if let Ok(mut d) = app.is_downloading.lock() { *d = false; }
                        if let Ok(mut dn) = app.download_done.lock() { *dn = false; }
                        app.refresh_bgm(); app.screen = Screen::BGM;
                    }
                    continue;
                }
                match app.screen {
                    Screen::Activity => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => { app.idx = app.idx.saturating_sub(1); l_state.select(Some(app.idx)); }
                        KeyCode::Down | KeyCode::Char('j') => { if app.idx < app.acts.len()-1 { app.idx += 1; l_state.select(Some(app.idx)); } }
                        KeyCode::Enter | KeyCode::Char('l') => app.screen = Screen::Duration,
                        KeyCode::Char('s') => app.screen = Screen::Settings,
                        KeyCode::Char('q') => break,
                        _ => {}
                    },
                    Screen::Duration => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => app.mins += 1,
                        KeyCode::Down | KeyCode::Char('j') => app.mins = app.mins.saturating_sub(1).max(1),
                        KeyCode::Enter | KeyCode::Char('l') => app.screen = Screen::Sessions,
                        KeyCode::Esc | KeyCode::Char('h') => app.screen = Screen::Activity,
                        _ => {}
                    },
                    Screen::Sessions => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => app.total += 1,
                        KeyCode::Down | KeyCode::Char('j') => app.total = app.total.saturating_sub(1).max(1),
                        KeyCode::Enter | KeyCode::Char('l') => { app.screen = Screen::BGM; l_state.select(Some(app.bgm_idx)); }
                        KeyCode::Esc | KeyCode::Char('h') => app.screen = Screen::Duration,
                        _ => {}
                    },
                    Screen::BGM => match key.code {
                        KeyCode::Char('i') => { app.screen = Screen::BGMImport; app.input.clear(); }
                        KeyCode::Up | KeyCode::Char('k') => { app.bgm_idx = app.bgm_idx.saturating_sub(1); l_state.select(Some(app.bgm_idx)); }
                        KeyCode::Down | KeyCode::Char('j') => { if app.bgm_idx < app.bgm_list.len()-1 { app.bgm_idx += 1; l_state.select(Some(app.bgm_idx)); } }
                        KeyCode::Enter | KeyCode::Char('l') => { app.rem = app.mins * 60; app.screen = Screen::Timer; app.work = true; app.paused = false; app.play_bgm(); }
                        KeyCode::Esc | KeyCode::Char('h') => app.screen = Screen::Sessions,
                        _ => {}
                    },
                    Screen::BGMImport => match key.code {
                        KeyCode::Enter => app.start_download(),
                        KeyCode::Char(c) => app.input.push(c),
                        KeyCode::Backspace => { app.input.pop(); }
                        KeyCode::Esc => app.screen = Screen::BGM,
                        _ => {}
                    },
                    Screen::Settings => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => app.settings_cursor = 0,
                        KeyCode::Down | KeyCode::Char('j') => app.settings_cursor = 1,
                        KeyCode::Left | KeyCode::Char('h') | KeyCode::Right | KeyCode::Char('l') => {
                            if app.settings_cursor == 0 { app.notifications_enabled = !app.notifications_enabled; }
                            else { app.theme = match app.theme { Theme::Cyan => Theme::Magenta, Theme::Magenta => Theme::Green, Theme::Green => Theme::Yellow, Theme::Yellow => Theme::Red, Theme::Red => Theme::Cyan }; }
                        }
                        KeyCode::Esc | KeyCode::Char('q') => app.screen = Screen::Activity,
                        _ => {}
                    },
                    Screen::Timer => match key.code {
                        KeyCode::Char(' ') => app.toggle_pause(),
                        KeyCode::Char('+') | KeyCode::Char('=') => app.adjust_volume(0.05),
                        KeyCode::Char('-') | KeyCode::Char('_') => app.adjust_volume(-0.05),
                        KeyCode::Char('q') | KeyCode::Esc | KeyCode::Char('h') => { app.stop_bgm(); app.screen = Screen::Activity; }
                        _ => {}
                    },
                }
            }
        }
        if app.tick.elapsed() >= Duration::from_secs(1) {
            app.on_tick(); update_presence(&mut drpc, &app); app.tick = Instant::now();
        }
    }
    disable_raw_mode()?; execute!(io::stdout(), LeaveAlternateScreen, DisableMouseCapture)?; Ok(())
}

fn ui(f: &mut ratatui::Frame, app: &mut App, l_state: &mut ListState) {
    let size = f.size();
    let theme_color = app.theme.color();
    let chunks = Layout::default().direction(Direction::Vertical).constraints([Constraint::Length(3), Constraint::Min(10), Constraint::Length(3)]).split(size);

    f.render_widget(Paragraph::new("POMODORO TUI").alignment(Alignment::Center).block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(theme_color))), chunks[0]);
    let main_area = centered_rect(70, 60, chunks[1]);

    // حل تحذير status_msg: نعرض الرسالة الآن بوضوح في الواجهة
    if *app.is_downloading.lock().unwrap() {
        let msg = app.status_msg.lock().unwrap().clone();
        f.render_widget(Paragraph::new(format!("\n\n{}", msg)).alignment(Alignment::Center).block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(theme_color)).title(" Background Process ")), main_area);
    } else {
        match app.screen {
            Screen::Activity => {
                let items: Vec<ListItem> = app.acts.iter().map(|a| ListItem::new(*a)).collect();
                f.render_stateful_widget(List::new(items).block(Block::default().title(" [1] Select Activity ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))).highlight_style(Style::default().bg(theme_color).fg(Color::Black)), main_area, l_state);
            }
            Screen::Duration => {
                f.render_widget(Paragraph::new(format!("\n\nFocus Time: {} min\n\n[J/K] Adjust | [L] Next", app.mins)).alignment(Alignment::Center).block(Block::default().title(" [2] Duration ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))), main_area);
            }
            Screen::Sessions => {
                f.render_widget(Paragraph::new(format!("\n\nSessions: {}\n\n[J/K] Adjust | [L] Next", app.total)).alignment(Alignment::Center).block(Block::default().title(" [3] Sessions ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))), main_area);
            }
            Screen::BGM => {
                let items: Vec<ListItem> = app.bgm_list.iter().map(|b| ListItem::new(b.as_str())).collect();
                f.render_stateful_widget(List::new(items).block(Block::default().title(" [4] Background Song (Press 'i' to Import) ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))).highlight_style(Style::default().bg(theme_color).fg(Color::Black)), main_area, l_state);
            }
            Screen::BGMImport => {
                let p = Paragraph::new(format!("\nPaste YouTube URL:\n{}\n\n[Enter] Download | [Esc] Cancel", app.input)).alignment(Alignment::Center).block(Block::default().title(" Import BGM ").borders(Borders::ALL).border_style(Style::default().fg(theme_color)));
                f.render_widget(p, main_area);
            }
            Screen::Settings => {
                let n_status = if app.notifications_enabled { "ON" } else { "OFF" };
                let text = vec![
                    Line::from(vec![Span::styled(if app.settings_cursor == 0 { "> Notifications: " } else { "  Notifications: " }, Style::default().fg(if app.settings_cursor == 0 { theme_color } else { Color::White })), Span::raw(n_status)]),
                    Line::from(vec![Span::raw("")]),
                    Line::from(vec![Span::styled(if app.settings_cursor == 1 { "> Theme: " } else { "  Theme: " }, Style::default().fg(if app.settings_cursor == 1 { theme_color } else { Color::White })), Span::raw(format!("{:?}", app.theme))]),
                ];
                f.render_widget(Paragraph::new(text).alignment(Alignment::Center).block(Block::default().title(" Settings ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))), main_area);
            }
            Screen::Timer => {
                let total = if app.work { app.mins * 60 } else { if app.mins >= 40 { 600 } else { 300 } };
                let pct = ((total - app.rem) as f64 / total as f64 * 100.0) as u16;
                let gauge_color = if app.paused { Color::Gray } else if app.work { Color::Red } else { Color::Green };
                let v_level = (app.volume * 100.0) as u32;
                f.render_widget(Gauge::default().block(Block::default().title(format!(" Session {} of {} ", app.current, app.total)).borders(Borders::ALL)).gauge_style(Style::default().fg(gauge_color)).percent(pct.min(100)).label(format!("{}:{:02} | Vol: {}%", app.rem / 60, app.rem % 60, v_level)), main_area);
            }
        }
    }
    f.render_widget(Paragraph::new(" [Space] Pause | [+/-] Vol | [HJKL] Navigate | [Q] Quit ").alignment(Alignment::Center).block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(Color::DarkGray))), chunks[2]);
}

fn centered_rect(percent_x: u16, percent_y: u16, r: Rect) -> Rect {
    let popup_layout = Layout::default().direction(Direction::Vertical).constraints([Constraint::Percentage((100 - percent_y) / 2), Constraint::Percentage(percent_y), Constraint::Percentage((100 - percent_y) / 2)]).split(r);
    Layout::default().direction(Direction::Horizontal).constraints([Constraint::Percentage((100 - percent_x) / 2), Constraint::Percentage(percent_x), Constraint::Percentage((100 - percent_x) / 2)]).split(popup_layout[1])[1]
}

fn update_presence(drpc: &mut Option<DiscordIpcClient>, app: &App) {
    if let Some(c) = drpc {
        let (state, details) = match app.screen {
            Screen::Timer => {
                let s = if app.paused { format!("⏸️ Paused: {}", app.acts[app.idx]) } 
                        else if !app.work { "☕ Taking a Break".to_string() }
                        else { format!("🔥 Focusing: {}", app.acts[app.idx]) };
                (s, format!("Session {} of {}", app.current, app.total))
            },
            _ => ("Configuring...".into(), "Main Menu".into()),
        };
        let mut p = activity::Activity::new().state(&state).details(&details).assets(activity::Assets::new().large_image("app_icon"));
        if app.screen == Screen::Timer && !app.paused {
            let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
            p = p.timestamps(activity::Timestamps::new().end((now + app.rem as u64) as i64));
        }
        let _ = c.set_activity(p);
    }
}
