use std::{error::Error, io, fs, process::{Command, Stdio}, sync::{Arc, Mutex}, time::{Duration, Instant, SystemTime, UNIX_EPOCH}, path::PathBuf};
use crossterm::{event::{self, DisableMouseCapture, Event, KeyCode}, execute, terminal::*};
use discord_rich_presence::{activity, DiscordIpc, DiscordIpcClient};
use ratatui::{backend::CrosstermBackend, layout::*, style::*, widgets::*, Terminal, text::{Line, Span}};
use rodio::{Decoder, OutputStream, Sink, Source};
use serde::{Serialize, Deserialize};

const APP_ID: &str = "1459887165784723673";
const EMBEDDED_SOUND: &[u8] = include_bytes!("../rain-sound.mp3");

// ─── Config & Data Types ────────────────────────────────────────────────────

#[derive(Debug, PartialEq, Clone, Copy, Serialize, Deserialize)]
enum Theme { Cyan, Magenta, Green, Yellow, Red }

impl Theme {
    fn color(&self) -> Color {
        match self {
            Theme::Cyan => Color::Cyan, Theme::Magenta => Color::Magenta,
            Theme::Green => Color::Green, Theme::Yellow => Color::Yellow, Theme::Red => Color::Red,
        }
    }
    fn all() -> &'static [Theme] {
        &[Theme::Cyan, Theme::Magenta, Theme::Green, Theme::Yellow, Theme::Red]
    }
    fn next(&self) -> Theme {
        let all = Self::all();
        let i = all.iter().position(|t| t == self).unwrap_or(0);
        all[(i + 1) % all.len()]
    }
}

#[derive(Debug, PartialEq, Clone)]
enum Screen { Activity, Duration, Sessions, BGM, BGMImport, Settings, Timer, Resume, Dashboard, DashboardDetail, AddActivity }

#[derive(Serialize, Deserialize, Clone)]
struct AppConfig {
    activities: Vec<String>,
    theme: Theme,
    notifications_enabled: bool,
    default_mins: u32,
    default_total: u32,
    volume: f32,
}

impl Default for AppConfig {
    fn default() -> Self {
        Self {
            activities: vec![
                "Studying".into(), "Coding".into(), "Deep Work".into(), "Reading".into(),
            ],
            theme: Theme::Cyan,
            notifications_enabled: true,
            default_mins: 25,
            default_total: 4,
            volume: 0.5,
        }
    }
}

#[derive(Serialize, Deserialize, Clone)]
struct SessionState {
    activity_name: String,
    activity_idx: usize,
    mins: u32,
    total: u32,
    current: u32,
    rem: u32,
    work: bool,
    bgm_idx: usize,
    volume: f32,
    muted: bool,
    saved_at: u64,
}

#[derive(Serialize, Deserialize, Clone, Debug)]
struct ProgressEntry {
    activity: String,
    duration_secs: u32,
    timestamp: u64,
    completed: bool,
}

#[derive(Serialize, Deserialize, Clone, Default)]
struct ProgressData {
    entries: Vec<ProgressEntry>,
}

#[derive(PartialEq, Clone, Copy)]
enum TimePeriod { All, Today, Week, Month }

impl TimePeriod {
    fn label(&self) -> &str {
        match self { TimePeriod::All => "All Time", TimePeriod::Today => "Today", TimePeriod::Week => "This Week", TimePeriod::Month => "This Month" }
    }
    fn next(&self) -> TimePeriod {
        match self { TimePeriod::All => TimePeriod::Today, TimePeriod::Today => TimePeriod::Week, TimePeriod::Week => TimePeriod::Month, TimePeriod::Month => TimePeriod::All }
    }
}

struct ActivityStats {
    name: String,
    total_secs: u64,
    sessions: usize,
    _completed_sessions: usize,
    last_session: Option<u64>,
    avg_secs: u64,
}

// ─── App State ──────────────────────────────────────────────────────────────

struct App {
    screen: Screen,
    prev_screen: Screen,
    acts: Vec<String>,
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
    muted: bool,
    data_dir: PathBuf,
    config: AppConfig,
    progress: ProgressData,
    time_period: TimePeriod,
    dashboard_idx: usize,
    resume_choice: bool,
    add_activity_input: String,
}

// ─── Persistence ────────────────────────────────────────────────────────────

fn config_path(data_dir: &PathBuf) -> PathBuf { data_dir.join("config.json") }
fn session_path(data_dir: &PathBuf) -> PathBuf { data_dir.join("session.json") }
fn progress_path(data_dir: &PathBuf) -> PathBuf { data_dir.join("progress.json") }

fn load_config(data_dir: &PathBuf) -> AppConfig {
    let path = config_path(data_dir);
    if let Ok(contents) = fs::read_to_string(&path) {
        if let Ok(cfg) = serde_json::from_str(&contents) { return cfg; }
    }
    AppConfig::default()
}

fn save_config(data_dir: &PathBuf, cfg: &AppConfig) {
    if let Ok(json) = serde_json::to_string_pretty(cfg) {
        let _ = fs::write(config_path(data_dir), json);
    }
}

fn load_session(data_dir: &PathBuf) -> Option<SessionState> {
    let path = session_path(data_dir);
    if let Ok(contents) = fs::read_to_string(&path) {
        if let Ok(s) = serde_json::from_str(&contents) { return Some(s); }
    }
    None
}

fn save_session(data_dir: &PathBuf, s: &SessionState) {
    if let Ok(json) = serde_json::to_string_pretty(s) {
        let _ = fs::write(session_path(data_dir), json);
    }
}

fn delete_session(data_dir: &PathBuf) {
    let _ = fs::remove_file(session_path(data_dir));
}

fn load_progress(data_dir: &PathBuf) -> ProgressData {
    let path = progress_path(data_dir);
    if let Ok(contents) = fs::read_to_string(&path) {
        if let Ok(p) = serde_json::from_str(&contents) { return p; }
    }
    ProgressData::default()
}

fn save_progress(data_dir: &PathBuf, p: &ProgressData) {
    if let Ok(json) = serde_json::to_string_pretty(p) {
        let _ = fs::write(progress_path(data_dir), json);
    }
}

fn now_epoch() -> u64 {
    SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_secs()
}

fn epoch_to_date(ts: u64) -> String {
    let secs = ts as i64;
    let days = secs / 86400;
    let day = days % 31 + 1;
    let month = (days / 31) % 12 + 1;
    let year = 1970 + days / 365;
    format!("{:04}-{:02}-{:02}", year, month, day)
}

fn is_in_period(ts: u64, period: TimePeriod) -> bool {
    let now = now_epoch();
    match period {
        TimePeriod::All => true,
        TimePeriod::Today => now.saturating_sub(ts) < 86400,
        TimePeriod::Week => now.saturating_sub(ts) < 604800,
        TimePeriod::Month => now.saturating_sub(ts) < 2592000,
    }
}

fn compute_stats(acts: &[String], progress: &ProgressData, period: TimePeriod) -> Vec<ActivityStats> {
    acts.iter().map(|name| {
        let entries: Vec<&ProgressEntry> = progress.entries.iter()
            .filter(|e| e.activity == *name && is_in_period(e.timestamp, period))
            .collect();
        let total_secs: u64 = entries.iter().map(|e| e.duration_secs as u64).sum();
        let sessions = entries.len();
        let completed = entries.iter().filter(|e| e.completed).count();
        let last_session = entries.iter().map(|e| e.timestamp).max();
        let avg_secs = if sessions > 0 { total_secs / sessions as u64 } else { 0 };
        ActivityStats { name: name.clone(), total_secs, sessions, _completed_sessions: completed, last_session, avg_secs }
    }).collect()
}

fn format_duration(secs: u64) -> String {
    let h = secs / 3600;
    let m = (secs % 3600) / 60;
    if h > 0 { format!("{}h {}m", h, m) } else { format!("{}m", m) }
}

// ─── App Implementation ─────────────────────────────────────────────────────

impl App {
    fn new() -> Self {
        let mut data_dir = dirs::data_dir().unwrap_or_else(|| PathBuf::from("."));
        data_dir.push("termdoro");
        let bgm_dir = data_dir.join("bgm");
        let _ = fs::create_dir_all(&bgm_dir);

        let rain_sound_path = bgm_dir.join("Rain_Background.mp3");
        if !rain_sound_path.exists() {
            let _ = fs::write(&rain_sound_path, EMBEDDED_SOUND);
        }

        let config = load_config(&data_dir);
        let progress = load_progress(&data_dir);
        let saved_session = load_session(&data_dir);

        let mut app = Self {
            screen: if saved_session.is_some() { Screen::Resume } else { Screen::Activity },
            prev_screen: Screen::Activity,
            acts: config.activities.clone(),
            idx: 0,
            mins: config.default_mins,
            total: config.default_total,
            current: 1,
            rem: config.default_mins * 60,
            work: true,
            paused: true,
            tick: Instant::now(),
            input: String::new(),
            status_msg: Arc::new(Mutex::new("Ready".into())),
            is_downloading: Arc::new(Mutex::new(false)),
            download_done: Arc::new(Mutex::new(false)),
            bgm_list: vec!["None".into()],
            bgm_idx: 0,
            sink: None,
            _stream: None,
            notifications_enabled: config.notifications_enabled,
            theme: config.theme,
            settings_cursor: 0,
            volume: config.volume,
            muted: false,
            data_dir,
            config,
            progress,
            time_period: TimePeriod::All,
            dashboard_idx: 0,
            resume_choice: true,
            add_activity_input: String::new(),
        };
        app.refresh_bgm();

        if let Some(ref s) = saved_session {
            app.idx = s.activity_idx.min(app.acts.len().saturating_sub(1));
            app.mins = s.mins;
            app.total = s.total;
            app.current = s.current;
            app.rem = s.rem;
            app.work = s.work;
            app.bgm_idx = s.bgm_idx.min(app.bgm_list.len().saturating_sub(1));
            app.volume = s.volume;
            app.muted = s.muted;
        }

        app
    }

    fn save_current_session(&self) {
        let s = SessionState {
            activity_name: self.acts[self.idx].clone(),
            activity_idx: self.idx,
            mins: self.mins,
            total: self.total,
            current: self.current,
            rem: self.rem,
            work: self.work,
            bgm_idx: self.bgm_idx,
            volume: self.volume,
            muted: self.muted,
            saved_at: now_epoch(),
        };
        save_session(&self.data_dir, &s);
    }

    fn clear_session(&self) {
        delete_session(&self.data_dir);
    }

    fn record_progress(&mut self, completed: bool) {
        let entry = ProgressEntry {
            activity: self.acts[self.idx].clone(),
            duration_secs: self.mins * 60,
            timestamp: now_epoch(),
            completed,
        };
        self.progress.entries.push(entry);
        save_progress(&self.data_dir, &self.progress);
    }

    fn save_config(&self) {
        let mut cfg = self.config.clone();
        cfg.activities = self.acts.clone();
        cfg.theme = self.theme;
        cfg.notifications_enabled = self.notifications_enabled;
        cfg.default_mins = self.mins;
        cfg.default_total = self.total;
        cfg.volume = self.volume;
        save_config(&self.data_dir, &cfg);
    }

    fn refresh_bgm(&mut self) {
        let mut list = vec!["None".into()];
        let bgm_dir = self.data_dir.join("bgm");
        if let Ok(entries) = fs::read_dir(bgm_dir) {
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
        let target_pattern = self.data_dir.join("bgm").join("%(title)s.%(ext)s");
        let target_str = target_pattern.to_string_lossy().into_owned();
        self.input.clear();

        std::thread::spawn(move || {
            if let Ok(mut d) = downloading.lock() { *d = true; }
            if let Ok(mut s) = status.lock() { *s = "Downloading to system storage...".into(); }
            
            let cmd = Command::new("yt-dlp")
                .args(["-x", "--audio-format", "mp3", "--quiet", "--no-warnings", "-o", &target_str, &url])
                .stdout(Stdio::null()).stderr(Stdio::null()).status();

            let msg = if let Ok(s) = cmd { if s.success() { "Success! Press ENTER" } else { "Failed" } } else { "yt-dlp missing" };
            if let Ok(mut s) = status.lock() { *s = msg.into(); }
            if let Ok(mut dn) = done.lock() { *dn = true; }
        });
    }

    fn play_bgm(&mut self) {
        self.stop_bgm();
        if self.bgm_idx == 0 || self.bgm_idx >= self.bgm_list.len() { return; }
        let path = self.data_dir.join("bgm").join(&self.bgm_list[self.bgm_idx]);
        if let Ok(file) = fs::File::open(path) {
            if let Ok((stream, handle)) = OutputStream::try_default() {
                if let Ok(sink) = Sink::try_new(&handle) {
                    if let Ok(source) = Decoder::new(io::BufReader::new(file)) {
                        sink.set_volume(if self.muted { 0.0 } else { self.volume });
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
        if !self.muted { if let Some(s) = &self.sink { s.set_volume(self.volume); } }
    }

    fn toggle_mute(&mut self) {
        self.muted = !self.muted;
        if let Some(s) = &self.sink { s.set_volume(if self.muted { 0.0 } else { self.volume }); }
    }

    fn toggle_pause(&mut self) {
        self.paused = !self.paused;
        if let Some(s) = &self.sink { if self.paused { s.pause(); } else { s.play(); } }
    }

    fn on_tick(&mut self) {
        if self.screen != Screen::Timer || self.paused || self.rem == 0 { return; }
        self.rem -= 1;
        if self.rem == 0 {
            let (t, b);
            if self.work {
                self.record_progress(true);
                if self.current >= self.total {
                    t = "Done!"; b = "All sessions finished!";
                    self.screen = Screen::Activity; self.stop_bgm(); self.clear_session(); self.save_config();
                } else {
                    self.work = false;
                    self.rem = if self.mins >= 40 { 600 } else { 300 };
                    t = "Break!"; b = "Time to rest.";
                }
            } else {
                self.work = true; self.current += 1; self.rem = self.mins * 60;
                t = "Work!"; b = "Focus time.";
            }
            if self.notifications_enabled { let _ = notify_rust::Notification::new().summary(t).body(b).show(); }
            self.paused = true;
            if let Some(s) = &self.sink { s.pause(); }
        }
    }

    fn start_timer(&mut self) {
        self.rem = self.mins * 60;
        self.screen = Screen::Timer;
        self.work = true;
        self.paused = false;
        self.play_bgm();
        self.save_current_session();
    }
}

// ─── Main ───────────────────────────────────────────────────────────────────

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
                    Screen::Resume => match key.code {
                        KeyCode::Up | KeyCode::Char('k') | KeyCode::Down | KeyCode::Char('j') => { app.resume_choice = !app.resume_choice; }
                        KeyCode::Enter => {
                            if app.resume_choice {
                                app.screen = Screen::Timer; app.paused = false; app.play_bgm();
                            } else {
                                app.clear_session();
                                app.mins = app.config.default_mins;
                                app.total = app.config.default_total;
                                app.current = 1; app.rem = app.mins * 60; app.work = true;
                                app.screen = Screen::Activity;
                            }
                        }
                        KeyCode::Char('q') => { app.clear_session(); break; }
                        _ => {}
                    },
                    Screen::Activity => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => { app.idx = app.idx.saturating_sub(1); l_state.select(Some(app.idx)); }
                        KeyCode::Down | KeyCode::Char('j') => { if app.idx < app.acts.len()-1 { app.idx += 1; l_state.select(Some(app.idx)); } }
                        KeyCode::Enter | KeyCode::Char('l') | KeyCode::Right => app.screen = Screen::Duration,
                        KeyCode::Char('s') => app.screen = Screen::Settings,
                        KeyCode::Char('d') => { app.screen = Screen::Dashboard; app.dashboard_idx = 0; }
                        KeyCode::Char('a') => { app.screen = Screen::AddActivity; app.add_activity_input.clear(); }
                        KeyCode::Char('q') => break,
                        _ => {}
                    },
                    Screen::Duration => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => app.mins += 1,
                        KeyCode::Down | KeyCode::Char('j') => app.mins = app.mins.saturating_sub(1).max(1),
                        KeyCode::Enter | KeyCode::Char('l') | KeyCode::Right => app.screen = Screen::Sessions,
                        KeyCode::Char('h') | KeyCode::Left | KeyCode::Esc => app.screen = Screen::Activity,
                        _ => {}
                    },
                    Screen::Sessions => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => app.total += 1,
                        KeyCode::Down | KeyCode::Char('j') => app.total = app.total.saturating_sub(1).max(1),
                        KeyCode::Enter | KeyCode::Char('l') | KeyCode::Right => { app.screen = Screen::BGM; l_state.select(Some(app.bgm_idx)); }
                        KeyCode::Char('h') | KeyCode::Left | KeyCode::Esc => app.screen = Screen::Duration,
                        _ => {}
                    },
                    Screen::BGM => match key.code {
                        KeyCode::Char('i') => { app.screen = Screen::BGMImport; app.input.clear(); }
                        KeyCode::Up | KeyCode::Char('k') => { app.bgm_idx = app.bgm_idx.saturating_sub(1); l_state.select(Some(app.bgm_idx)); }
                        KeyCode::Down | KeyCode::Char('j') => { if app.bgm_idx < app.bgm_list.len()-1 { app.bgm_idx += 1; l_state.select(Some(app.bgm_idx)); } }
                        KeyCode::Enter | KeyCode::Char('l') | KeyCode::Right => { app.start_timer(); }
                        KeyCode::Char('h') | KeyCode::Left | KeyCode::Esc => app.screen = Screen::Sessions,
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
                            else { app.theme = app.theme.next(); }
                        }
                        KeyCode::Esc | KeyCode::Char('q') => { app.save_config(); app.screen = Screen::Activity; }
                        _ => {}
                    },
                    Screen::Timer => match key.code {
                        KeyCode::Char(' ') => app.toggle_pause(),
                        KeyCode::Char('m') | KeyCode::Char('M') => app.toggle_mute(),
                        KeyCode::Char('+') | KeyCode::Char('=') => app.adjust_volume(0.05),
                        KeyCode::Char('-') | KeyCode::Char('_') => app.adjust_volume(-0.05),
                        KeyCode::Char('q') | KeyCode::Char('h') | KeyCode::Left | KeyCode::Esc => {
                            app.stop_bgm(); app.save_current_session(); app.screen = Screen::Activity;
                        }
                        _ => {}
                    },
                    Screen::Dashboard => match key.code {
                        KeyCode::Up | KeyCode::Char('k') => { app.dashboard_idx = app.dashboard_idx.saturating_sub(1); }
                        KeyCode::Down | KeyCode::Char('j') => {
                            let stats = compute_stats(&app.acts, &app.progress, app.time_period);
                            if app.dashboard_idx < stats.len().saturating_sub(1) { app.dashboard_idx += 1; }
                        }
                        KeyCode::Left | KeyCode::Char('h') => { app.time_period = match app.time_period { TimePeriod::Today => TimePeriod::All, TimePeriod::Week => TimePeriod::Today, TimePeriod::Month => TimePeriod::Week, TimePeriod::All => TimePeriod::Month }; app.dashboard_idx = 0; }
                        KeyCode::Right | KeyCode::Char('l') => { app.time_period = app.time_period.next(); app.dashboard_idx = 0; }
                        KeyCode::Enter => { app.prev_screen = Screen::Dashboard; app.screen = Screen::DashboardDetail; }
                        KeyCode::Esc | KeyCode::Char('q') => app.screen = Screen::Activity,
                        _ => {}
                    },
                    Screen::DashboardDetail => match key.code {
                        KeyCode::Esc | KeyCode::Char('q') | KeyCode::Char('h') => app.screen = Screen::Dashboard,
                        _ => {}
                    },
                    Screen::AddActivity => match key.code {
                        KeyCode::Char(c) => app.add_activity_input.push(c),
                        KeyCode::Backspace => { app.add_activity_input.pop(); }
                        KeyCode::Enter => {
                            let name = app.add_activity_input.trim().to_string();
                            if !name.is_empty() && !app.acts.contains(&name) {
                                app.acts.push(name);
                                app.config.activities = app.acts.clone();
                                app.save_config();
                            }
                            app.screen = Screen::Activity;
                        }
                        KeyCode::Esc => app.screen = Screen::Activity,
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

// ─── UI Rendering ───────────────────────────────────────────────────────────

fn ui(f: &mut ratatui::Frame, app: &mut App, l_state: &mut ListState) {
    let size = f.size();
    let theme_color = app.theme.color();
    let chunks = Layout::default().direction(Direction::Vertical).constraints([
        Constraint::Length(3), Constraint::Min(10), Constraint::Length(3)
    ]).split(size);

    f.render_widget(
        Paragraph::new("TERMDORO").alignment(Alignment::Center)
            .block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
        chunks[0]
    );

    let main_area = centered_rect(70, 60, chunks[1]);

    if *app.is_downloading.lock().unwrap() {
        let msg = app.status_msg.lock().unwrap().clone();
        f.render_widget(
            Paragraph::new(format!("\n\n{}", msg)).alignment(Alignment::Center)
                .block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(theme_color)).title(" Background Process ")),
            main_area
        );
    } else {
        match app.screen {
            Screen::Resume => {
                let text = vec![
                    Line::from(""),
                    Line::from(Span::styled("  You have a saved session!", Style::default().fg(theme_color).add_modifier(Modifier::BOLD))),
                    Line::from(""),
                    Line::from(vec![
                        Span::styled(if app.resume_choice { " > " } else { "   " }, Style::default().fg(theme_color)),
                        Span::raw("Resume last session"),
                    ]),
                    Line::from(vec![
                        Span::styled(if !app.resume_choice { " > " } else { "   " }, Style::default().fg(theme_color)),
                        Span::raw("Start fresh"),
                    ]),
                    Line::from(""),
                    Line::from(Span::styled("  [J/K] Select  [Enter] Confirm  [Q] Quit", Style::default().fg(Color::DarkGray))),
                ];
                f.render_widget(
                    Paragraph::new(text).alignment(Alignment::Left)
                        .block(Block::default().title(" Welcome Back ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
            Screen::Activity => {
                let mut items: Vec<ListItem> = app.acts.iter().map(|a| ListItem::new(a.as_str())).collect();
                items.push(ListItem::new(Span::styled("+ Add Activity", Style::default().fg(Color::DarkGray))));
                l_state.select(Some(app.idx));
                f.render_stateful_widget(
                    List::new(items)
                        .block(Block::default().title(" [1] Select Activity ").borders(Borders::ALL).border_style(Style::default().fg(theme_color)))
                        .highlight_style(Style::default().bg(theme_color).fg(Color::Black)),
                    main_area, l_state
                );
            }
            Screen::Duration => {
                f.render_widget(
                    Paragraph::new(format!("\n\nFocus Time: {} min\n\n[J/K] Adjust | [L/Right] Next", app.mins))
                        .alignment(Alignment::Center)
                        .block(Block::default().title(" [2] Duration ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
            Screen::Sessions => {
                f.render_widget(
                    Paragraph::new(format!("\n\nSessions: {}\n\n[J/K] Adjust | [L/Right] Next", app.total))
                        .alignment(Alignment::Center)
                        .block(Block::default().title(" [3] Sessions ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
            Screen::BGM => {
                let items: Vec<ListItem> = app.bgm_list.iter().map(|b| ListItem::new(b.as_str())).collect();
                l_state.select(Some(app.bgm_idx));
                f.render_stateful_widget(
                    List::new(items)
                        .block(Block::default().title(" [4] Background Song (Press 'i' to Import) ").borders(Borders::ALL).border_style(Style::default().fg(theme_color)))
                        .highlight_style(Style::default().bg(theme_color).fg(Color::Black)),
                    main_area, l_state
                );
            }
            Screen::BGMImport => {
                f.render_widget(
                    Paragraph::new(format!("\nPaste YouTube URL:\n{}\n\n[Enter] Download | [Esc] Cancel", app.input))
                        .alignment(Alignment::Center)
                        .block(Block::default().title(" Import BGM ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
            Screen::Settings => {
                let n_status = if app.notifications_enabled { "ON" } else { "OFF" };
                let t_name = format!("{:?}", app.theme);
                let text = vec![
                    Line::from(vec![
                        Span::styled(if app.settings_cursor == 0 { "> Notifications: " } else { "  Notifications: " },
                            Style::default().fg(if app.settings_cursor == 0 { theme_color } else { Color::White })),
                        Span::raw(n_status)
                    ]),
                    Line::from(""),
                    Line::from(vec![
                        Span::styled(if app.settings_cursor == 1 { "> Theme: " } else { "  Theme: " },
                            Style::default().fg(if app.settings_cursor == 1 { theme_color } else { Color::White })),
                        Span::raw(&t_name)
                    ]),
                ];
                f.render_widget(
                    Paragraph::new(text).alignment(Alignment::Center)
                        .block(Block::default().title(" Settings ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
            Screen::Timer => {
                let total = if app.work { app.mins * 60 } else { if app.mins >= 40 { 600 } else { 300 } };
                let pct = if total > 0 { ((total - app.rem) as f64 / total as f64 * 100.0) as u16 } else { 0 };
                let gauge_color = if app.paused { Color::Gray } else if app.work { Color::Red } else { Color::Green };
                let v_level = if app.muted { "Muted".to_string() } else { format!("{}%", (app.volume * 100.0) as u32) };
                let activity = app.acts.get(app.idx).map(|s| s.as_str()).unwrap_or("");
                let label = format!("{}:{:02} | Vol: {} | {}", app.rem / 60, app.rem % 60, v_level, activity);
                f.render_widget(
                    Gauge::default()
                        .block(Block::default().title(format!(" Session {} of {} ", app.current, app.total)).borders(Borders::ALL))
                        .gauge_style(Style::default().fg(gauge_color))
                        .percent(pct.min(100))
                        .label(label),
                    main_area
                );
            }
            Screen::Dashboard => {
                let stats = compute_stats(&app.acts, &app.progress, app.time_period);
                let period_label = app.time_period.label();
                let total_focus: u64 = stats.iter().map(|s| s.total_secs).sum();
                let total_sessions: usize = stats.iter().map(|s| s.sessions).sum();

                let mut lines: Vec<Line> = vec![];
                lines.push(Line::from(vec![
                    Span::styled("  Period: ", Style::default().fg(Color::White).add_modifier(Modifier::BOLD)),
                    Span::styled(period_label, Style::default().fg(theme_color).add_modifier(Modifier::BOLD)),
                    Span::raw("  [H/L] Change"),
                ]));
                lines.push(Line::from(vec![
                    Span::styled("  Total: ", Style::default().fg(Color::White)),
                    Span::styled(format!("{} focused across {} sessions", format_duration(total_focus), total_sessions), Style::default().fg(theme_color)),
                ]));
                lines.push(Line::from(""));

                for (i, stat) in stats.iter().enumerate() {
                    let marker = if i == app.dashboard_idx { "> " } else { "  " };
                    let style = if i == app.dashboard_idx { Style::default().fg(theme_color).add_modifier(Modifier::BOLD) } else { Style::default().fg(Color::White) };
                    lines.push(Line::from(vec![
                        Span::styled(marker, style),
                        Span::styled(&stat.name, style),
                        Span::raw("  "),
                        Span::styled(format_duration(stat.total_secs), Style::default().fg(Color::DarkGray)),
                        Span::raw(format!("  {} sessions", stat.sessions)),
                    ]));
                }

                if stats.is_empty() {
                    lines.push(Line::from(Span::styled("  No data yet. Start a focus session!", Style::default().fg(Color::DarkGray))));
                }

                f.render_widget(
                    Paragraph::new(lines)
                        .block(Block::default().title(" Dashboard [D] ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
            Screen::DashboardDetail => {
                let stats = compute_stats(&app.acts, &app.progress, app.time_period);
                if let Some(stat) = stats.get(app.dashboard_idx) {
                    let last_date = stat.last_session.map(|ts| epoch_to_date(ts)).unwrap_or_else(|| "Never".into());
                    let entries: Vec<&ProgressEntry> = app.progress.entries.iter()
                        .filter(|e| e.activity == stat.name && is_in_period(e.timestamp, app.time_period))
                        .collect();
                    let completed = entries.iter().filter(|e| e.completed).count();
                    let incomplete = entries.len().saturating_sub(completed);

                    let mut lines: Vec<Line> = vec![];
                    lines.push(Line::from(vec![
                        Span::styled(format!("  {}", stat.name), Style::default().fg(theme_color).add_modifier(Modifier::BOLD)),
                    ]));
                    lines.push(Line::from(""));
                    lines.push(Line::from(vec![
                        Span::styled("  Total Focus Time:  ", Style::default().fg(Color::White)),
                        Span::styled(format_duration(stat.total_secs), Style::default().fg(theme_color).add_modifier(Modifier::BOLD)),
                    ]));
                    lines.push(Line::from(vec![
                        Span::styled("  Total Sessions:    ", Style::default().fg(Color::White)),
                        Span::raw(format!("{}", stat.sessions)),
                    ]));
                    lines.push(Line::from(vec![
                        Span::styled("  Completed:         ", Style::default().fg(Color::White)),
                        Span::styled(format!("{}", completed), Style::default().fg(Color::Green)),
                    ]));
                    if incomplete > 0 {
                        lines.push(Line::from(vec![
                            Span::styled("  Incomplete:        ", Style::default().fg(Color::White)),
                            Span::styled(format!("{}", incomplete), Style::default().fg(Color::Yellow)),
                        ]));
                    }
                    lines.push(Line::from(vec![
                        Span::styled("  Average Session:   ", Style::default().fg(Color::White)),
                        Span::raw(format_duration(stat.avg_secs)),
                    ]));
                    lines.push(Line::from(vec![
                        Span::styled("  Last Session:      ", Style::default().fg(Color::White)),
                        Span::raw(&last_date),
                    ]));
                    lines.push(Line::from(""));
                    lines.push(Line::from(Span::styled("  [Esc] Back", Style::default().fg(Color::DarkGray))));

                    f.render_widget(
                        Paragraph::new(lines)
                            .block(Block::default().title(format!(" {} - Details ", stat.name)).borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                        main_area
                    );
                }
            }
            Screen::AddActivity => {
                f.render_widget(
                    Paragraph::new(format!("\nNew Activity Name:\n{}\n\n[Enter] Add  [Esc] Cancel", app.add_activity_input))
                        .alignment(Alignment::Center)
                        .block(Block::default().title(" Add Activity ").borders(Borders::ALL).border_style(Style::default().fg(theme_color))),
                    main_area
                );
            }
        }
    }

    let help_text = match app.screen {
        Screen::Activity => " [J/K] Move | [A] Add Activity | [D] Dashboard | [S] Settings | [Q] Quit ",
        Screen::Timer => " [Space] Pause | [+/-] Vol | [M] Mute | [Esc] Stop & Save ",
        Screen::Settings => " [J/K] Select | [H/L] Change | [Esc] Back ",
        Screen::Dashboard => " [J/K] Select | [H/L] Period | [Enter] Detail | [Esc] Back ",
        Screen::Resume => " [J/K] Select | [Enter] Confirm | [Q] Quit ",
        Screen::AddActivity => " Type name | [Enter] Add | [Esc] Cancel ",
        _ => " [HJKL] Navigate | [Esc] Back ",
    };
    f.render_widget(
        Paragraph::new(help_text).alignment(Alignment::Center)
            .block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(Color::DarkGray))),
        chunks[2]
    );
}

fn centered_rect(percent_x: u16, percent_y: u16, r: Rect) -> Rect {
    let popup_layout = Layout::default().direction(Direction::Vertical).constraints([
        Constraint::Percentage((100 - percent_y) / 2), Constraint::Percentage(percent_y), Constraint::Percentage((100 - percent_y) / 2)
    ]).split(r);
    Layout::default().direction(Direction::Horizontal).constraints([
        Constraint::Percentage((100 - percent_x) / 2), Constraint::Percentage(percent_x), Constraint::Percentage((100 - percent_x) / 2)
    ]).split(popup_layout[1])[1]
}

// ─── Discord Rich Presence ──────────────────────────────────────────────────

fn update_presence(drpc: &mut Option<DiscordIpcClient>, app: &App) {
    if let Some(c) = drpc {
        let (state, details) = match app.screen {
            Screen::Timer => {
                let activity = app.acts.get(app.idx).map(|s| s.as_str()).unwrap_or("Unknown");
                if app.paused {
                    (format!("Paused: {}", activity), format!("Session {} of {}", app.current, app.total))
                } else if !app.work {
                    ("Taking a Break".into(), format!("Session {} of {}", app.current, app.total))
                } else {
                    (format!("Focusing: {}", activity), format!("Session {} of {}", app.current, app.total))
                }
            }
            _ => ("Configuring...".into(), "Main Menu".into()),
        };
        let mut p = activity::Activity::new()
            .state(&state)
            .details(&details)
            .assets(activity::Assets::new()
                .large_image("app_icon")
                .large_text("Termdoro"));
        if app.screen == Screen::Timer && !app.paused && app.work {
            let now = now_epoch();
            p = p.timestamps(activity::Timestamps::new().end((now + app.rem as u64) as i64));
        }
        let _ = c.set_activity(p);
    }
}
