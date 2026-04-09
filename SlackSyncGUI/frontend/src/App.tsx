import { useState } from 'react';
import './App.css';

type ViewMode = 'login' | 'dashboard';
type SyncMode = 'channel' | 'url';

interface Channel {
  id: string;
  name: string;
  topic: string;
  member_count: number;
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          Login: (workspace: string, cookie: string) => Promise<string>;
          LoginWithBrowser: (workspace: string) => Promise<string>;
          GetChannels: () => Promise<Channel[]>;
          StartSync: (ids: string[], path: string, formats: string[]) => Promise<void>;
          StartSyncURLs: (urls: string[], path: string, formats: string[]) => Promise<void>;
          SelectOutputDirectory: () => Promise<string>;
        };
      };
    };
  }
}

const exportFormats = [
  { id: 'html', label: 'HTML', description: '适合直接浏览和分享' },
  { id: 'markdown', label: 'Markdown', description: '适合沉淀到知识库' },
  { id: 'pdf', label: 'PDF', description: '适合归档和打印' },
  { id: 'json', label: 'JSON', description: '适合做程序化处理' },
  { id: 'sqlite', label: 'SQLite', description: '适合本地检索和分析' },
] as const;

function getAppBridge() {
  return window.go?.main?.App;
}

async function login(workspace: string, cookie: string) {
  const bridge = getAppBridge();
  if (!bridge?.Login) {
    throw new Error('后端未连接');
  }
  return bridge.Login(workspace, cookie);
}

async function loginWithBrowser(workspace: string) {
  const bridge = getAppBridge();
  if (!bridge?.LoginWithBrowser) {
    throw new Error('后端未连接');
  }
  return bridge.LoginWithBrowser(workspace);
}

async function getChannels() {
  const bridge = getAppBridge();
  if (!bridge?.GetChannels) {
    throw new Error('后端未连接');
  }
  return bridge.GetChannels();
}

async function startSync(ids: string[], path: string, formats: string[]) {
  const bridge = getAppBridge();
  if (!bridge?.StartSync) {
    throw new Error('后端未连接');
  }
  return bridge.StartSync(ids, path, formats);
}

async function startSyncURLs(urls: string[], path: string, formats: string[]) {
  const bridge = getAppBridge();
  if (!bridge?.StartSyncURLs) {
    throw new Error('后端未连接');
  }
  return bridge.StartSyncURLs(urls, path, formats);
}

async function selectOutputDirectory() {
  const bridge = getAppBridge();
  if (!bridge?.SelectOutputDirectory) {
    throw new Error('后端未连接');
  }
  return bridge.SelectOutputDirectory();
}

function App() {
  const [view, setView] = useState<ViewMode>('login');
  const [workspace, setWorkspace] = useState('');
  const [cookie, setCookie] = useState('');
  const [error, setError] = useState('');
  const [channels, setChannels] = useState<Channel[]>([]);
  const [selectedChannels, setSelectedChannels] = useState<Set<string>>(new Set());
  const [logs, setLogs] = useState<string[]>([
    '准备就绪。登录后可以按频道同步，或直接粘贴 Slack 线程链接进行抓取。',
  ]);
  const [isSyncing, setIsSyncing] = useState(false);
  const [isBrowserLoading, setIsBrowserLoading] = useState(false);
  const [syncMode, setSyncMode] = useState<SyncMode>('channel');
  const [urlList, setUrlList] = useState('');
  const [exportPath, setExportPath] = useState('');
  const [formats, setFormats] = useState<Set<string>>(new Set(['html']));

  const appendLog = (message: string) => {
    const stamp = new Date().toLocaleTimeString();
    setLogs((current) => [...current, `[${stamp}] ${message}`]);
  };

  const handleLogin = async () => {
    if (!workspace.trim() || !cookie.trim()) {
      setError('请先填写工作区和 Cookie。');
      return;
    }

    try {
      setError('');
      appendLog('正在建立会话...');
      const user = await login(workspace.trim(), cookie.trim());
      appendLog(`认证成功，当前账号：${user}`);
      const nextChannels = await getChannels();
      setChannels(nextChannels);
      appendLog(`已加载 ${nextChannels.length} 个频道。`);
      setView('dashboard');
    } catch (err) {
      setError(`登录失败：${String(err)}`);
      appendLog(`登录失败：${String(err)}`);
    }
  };

  const handleBrowserLogin = async () => {
    if (!workspace.trim()) {
      setError('请先输入工作区，支持 myteam、myteam.slack.com 或完整 URL。');
      return;
    }

    try {
      setIsBrowserLoading(true);
      setError('');
      appendLog('正在打开浏览器，请完成 Slack 登录...');
      const nextCookie = await loginWithBrowser(workspace.trim());
      if (nextCookie) {
        setCookie(nextCookie);
        appendLog('已自动获取 d Cookie。');
      }
    } catch (err) {
      setError(`浏览器登录失败：${String(err)}`);
      appendLog(`浏览器登录失败：${String(err)}`);
    } finally {
      setIsBrowserLoading(false);
    }
  };

  const toggleChannel = (id: string) => {
    setSelectedChannels((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const toggleFormat = (id: string) => {
    setFormats((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const chooseDirectory = async () => {
    try {
      const path = await selectOutputDirectory();
      if (path) {
        setExportPath(path);
        appendLog(`输出目录已更新：${path}`);
      }
    } catch (err) {
      setError(`选择目录失败：${String(err)}`);
    }
  };

  const handleSync = async () => {
    const selectedFormats = Array.from(formats);
    const path = exportPath || '.';

    if (selectedFormats.length === 0) {
      setError('至少选择一种导出格式。');
      return;
    }

    if (syncMode === 'channel' && selectedChannels.size === 0) {
      setError('请先选择至少一个频道。');
      return;
    }

    if (syncMode === 'url' && !urlList.trim()) {
      setError('请先粘贴至少一个 Slack 链接。');
      return;
    }

    try {
      setError('');
      setIsSyncing(true);
      appendLog(`开始同步，输出目录：${path}`);
      appendLog(`导出格式：${selectedFormats.join(', ')}`);

      if (syncMode === 'channel') {
        const ids = Array.from(selectedChannels);
        appendLog(`按频道同步，共 ${ids.length} 个频道。`);
        await startSync(ids, path, selectedFormats);
      } else {
        const urls = urlList
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean);
        appendLog(`按 URL 同步，共 ${urls.length} 条链接。`);
        await startSyncURLs(urls, path, selectedFormats);
      }

      appendLog('同步完成。');
      if (formats.has('sqlite')) {
        appendLog('SQLite 数据库会写入 slackdump.db。');
      }
    } catch (err) {
      setError(`同步失败：${String(err)}`);
      appendLog(`同步失败：${String(err)}`);
    } finally {
      setIsSyncing(false);
    }
  };

  if (view === 'login') {
    return (
      <div className="min-h-screen bg-[radial-gradient(circle_at_top_left,_rgba(244,186,94,0.18),_transparent_30%),linear-gradient(135deg,_#f7f2ea,_#f3efe7_45%,_#efe9df)] px-6 py-10 text-slate-800">
        <div className="mx-auto flex min-h-[calc(100vh-5rem)] max-w-6xl items-center gap-8 lg:flex-row">
          <section className="flex-1">
            <p className="mb-4 inline-flex rounded-full border border-amber-200 bg-white/70 px-4 py-1 text-sm font-medium text-amber-700 backdrop-blur">
              Desktop GUI built on top of slackdump
            </p>
            <h1 className="max-w-3xl text-5xl font-black tracking-tight text-slate-900">
              slack同步器
            </h1>
            <p className="mt-5 max-w-2xl text-lg leading-8 text-slate-600">
              面向个人归档和内容整理的 Slack 桌面同步工具。你可以按频道批量抓取，也可以
              直接贴线程链接，把内容落到本地的 HTML、Markdown、PDF、JSON 或 SQLite。
            </p>
            <div className="mt-8 grid gap-4 sm:grid-cols-3">
              <div className="rounded-3xl border border-white/70 bg-white/75 p-5 shadow-[0_20px_50px_rgba(148,110,52,0.08)] backdrop-blur">
                <p className="text-sm font-semibold text-slate-500">同步模式</p>
                <p className="mt-3 text-lg font-bold text-slate-900">频道 / URL 双入口</p>
              </div>
              <div className="rounded-3xl border border-white/70 bg-white/75 p-5 shadow-[0_20px_50px_rgba(148,110,52,0.08)] backdrop-blur">
                <p className="text-sm font-semibold text-slate-500">输出格式</p>
                <p className="mt-3 text-lg font-bold text-slate-900">HTML / MD / PDF / JSON / SQLite</p>
              </div>
              <div className="rounded-3xl border border-white/70 bg-white/75 p-5 shadow-[0_20px_50px_rgba(148,110,52,0.08)] backdrop-blur">
                <p className="text-sm font-semibold text-slate-500">适用场景</p>
                <p className="mt-3 text-lg font-bold text-slate-900">归档、复盘、知识沉淀</p>
              </div>
            </div>
          </section>

          <section className="w-full max-w-xl rounded-[2rem] border border-white/70 bg-white/88 p-8 shadow-[0_30px_80px_rgba(119,87,45,0.12)] backdrop-blur">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-slate-400">登录</p>
            <h2 className="mt-3 text-3xl font-black text-slate-900">连接你的 Slack 工作区</h2>
            <p className="mt-3 text-sm leading-7 text-slate-500">
              支持 `myteam`、`myteam.slack.com` 或完整 `https://myteam.slack.com`。
            </p>

            <div className="mt-8 space-y-5">
              <label className="block">
                <span className="mb-2 block text-sm font-semibold text-slate-600">Workspace</span>
                <input
                  value={workspace}
                  onChange={(event) => setWorkspace(event.target.value)}
                  placeholder="myteam.slack.com"
                  className="w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-base text-slate-800 outline-none transition focus:border-amber-300 focus:bg-white focus:ring-4 focus:ring-amber-100"
                />
              </label>

              <label className="block">
                <span className="mb-2 block text-sm font-semibold text-slate-600">Cookie (d)</span>
                <input
                  value={cookie}
                  onChange={(event) => setCookie(event.target.value)}
                  placeholder="xoxd-..."
                  type="password"
                  className="w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm font-mono text-slate-800 outline-none transition focus:border-amber-300 focus:bg-white focus:ring-4 focus:ring-amber-100"
                />
              </label>
            </div>

            {error ? (
              <div className="mt-5 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-medium text-rose-700">
                {error}
              </div>
            ) : null}

            <div className="mt-7 flex flex-col gap-3 sm:flex-row">
              <button
                onClick={handleLogin}
                disabled={!workspace.trim() || !cookie.trim()}
                className="flex-1 rounded-2xl bg-slate-900 px-5 py-3 text-base font-bold text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300"
              >
                使用 Cookie 登录
              </button>
              <button
                onClick={handleBrowserLogin}
                disabled={isBrowserLoading || !workspace.trim()}
                className="flex-1 rounded-2xl border border-amber-300 bg-amber-50 px-5 py-3 text-base font-bold text-amber-900 transition hover:bg-amber-100 disabled:cursor-not-allowed disabled:border-slate-200 disabled:bg-slate-100 disabled:text-slate-400"
              >
                {isBrowserLoading ? '等待浏览器登录...' : '浏览器辅助登录'}
              </button>
            </div>

            <div className="mt-6 rounded-2xl bg-slate-50 px-4 py-4 text-sm leading-7 text-slate-500">
              浏览器辅助登录会打开你的 Slack 工作区页面，并尝试自动捕获 `d` Cookie。
              如果你已经手动取到了 Cookie，也可以直接粘贴后登录。
            </div>
          </section>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[linear-gradient(180deg,_#f4efe7,_#f7f4ee_24%,_#fbfaf7)] px-5 py-5 text-slate-800">
      <div className="mx-auto grid min-h-[calc(100vh-2.5rem)] max-w-[1500px] gap-5 xl:grid-cols-[340px_minmax(0,1fr)]">
        <aside className={`rounded-[2rem] border border-white/70 bg-white/86 p-5 shadow-[0_24px_60px_rgba(127,92,46,0.09)] backdrop-blur ${syncMode === 'url' ? 'opacity-60' : ''}`}>
          <div className="rounded-[1.5rem] bg-slate-900 px-5 py-5 text-white">
            <p className="text-xs uppercase tracking-[0.24em] text-slate-300">Workspace</p>
            <h2 className="mt-3 text-2xl font-black">{workspace || 'Slack Workspace'}</h2>
            <p className="mt-3 text-sm text-slate-300">当前已加载 {channels.length} 个频道。</p>
          </div>

          <div className="mt-5 flex items-center justify-between text-sm">
            <span className="font-medium text-slate-500">已选 {selectedChannels.size} 个频道</span>
            <div className="flex gap-2">
              <button
                onClick={() => setSelectedChannels(new Set(channels.map((channel) => channel.id)))}
                className="rounded-full bg-slate-100 px-3 py-1 font-medium text-slate-600 transition hover:bg-slate-200"
              >
                全选
              </button>
              <button
                onClick={() => setSelectedChannels(new Set())}
                className="rounded-full bg-slate-100 px-3 py-1 font-medium text-slate-600 transition hover:bg-slate-200"
              >
                清空
              </button>
            </div>
          </div>

          <div className="soft-scroll mt-4 max-h-[calc(100vh-18rem)] space-y-3 overflow-y-auto pr-1">
            {channels.map((channel) => {
              const selected = selectedChannels.has(channel.id);
              return (
                <button
                  key={channel.id}
                  onClick={() => toggleChannel(channel.id)}
                  className={`w-full rounded-[1.4rem] border px-4 py-4 text-left transition ${
                    selected
                      ? 'border-amber-300 bg-amber-50 shadow-[0_14px_36px_rgba(216,168,74,0.18)]'
                      : 'border-slate-200 bg-slate-50 hover:border-slate-300 hover:bg-white'
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-base font-bold text-slate-900">#{channel.name}</p>
                      <p className="mt-2 line-clamp-2 text-sm leading-6 text-slate-500">
                        {channel.topic || '暂无频道描述'}
                      </p>
                    </div>
                    <span className={`rounded-full px-2 py-1 text-xs font-semibold ${selected ? 'bg-amber-200 text-amber-900' : 'bg-white text-slate-500'}`}>
                      {channel.member_count}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>
        </aside>

        <main className="grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_380px]">
          <section className="space-y-5 rounded-[2rem] border border-white/70 bg-white/88 p-6 shadow-[0_24px_60px_rgba(127,92,46,0.09)] backdrop-blur">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <p className="text-sm font-semibold uppercase tracking-[0.24em] text-slate-400">控制台</p>
                <h1 className="mt-2 text-4xl font-black text-slate-900">开始同步</h1>
                <p className="mt-3 max-w-2xl text-sm leading-7 text-slate-500">
                  你可以按频道批量抓取，也可以粘贴 Slack 帖子链接按线程提取上下文。
                </p>
              </div>

              <div className="inline-flex rounded-2xl border border-slate-200 bg-slate-50 p-1">
                <button
                  onClick={() => setSyncMode('channel')}
                  className={`rounded-[1rem] px-4 py-2 text-sm font-semibold transition ${
                    syncMode === 'channel' ? 'bg-slate-900 text-white' : 'text-slate-500 hover:text-slate-800'
                  }`}
                >
                  按频道同步
                </button>
                <button
                  onClick={() => setSyncMode('url')}
                  className={`rounded-[1rem] px-4 py-2 text-sm font-semibold transition ${
                    syncMode === 'url' ? 'bg-slate-900 text-white' : 'text-slate-500 hover:text-slate-800'
                  }`}
                >
                  按 URL 抓取
                </button>
              </div>
            </div>

            {syncMode === 'url' ? (
              <div className="rounded-[1.5rem] border border-amber-200 bg-[linear-gradient(135deg,_rgba(255,248,236,0.95),_rgba(255,255,255,0.95))] p-5">
                <p className="text-sm font-semibold text-slate-700">Slack 链接列表</p>
                <p className="mt-2 text-sm leading-7 text-slate-500">
                  每行一个链接，支持频道消息和线程链接。适合绕过频道列表，直接抓某一批重点帖子。
                </p>
                <textarea
                  value={urlList}
                  onChange={(event) => setUrlList(event.target.value)}
                  placeholder={'https://app.slack.com/client/T000/C000/thread/C000-1670000000000000\nhttps://myteam.slack.com/archives/C123456/p1679654279815259'}
                  className="mt-4 min-h-[220px] w-full rounded-[1.4rem] border border-amber-200 bg-white px-4 py-4 font-mono text-sm leading-7 text-slate-700 outline-none transition focus:border-amber-300 focus:ring-4 focus:ring-amber-100"
                />
              </div>
            ) : (
              <div className="rounded-[1.5rem] border border-slate-200 bg-slate-50 p-5 text-sm leading-7 text-slate-500">
                左侧选择要同步的频道。同步时会尽量抓取线程内容，并将结果按标题落盘。
              </div>
            )}

            <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
              <div className="rounded-[1.5rem] border border-slate-200 bg-slate-50 p-5">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="text-sm font-semibold text-slate-700">导出目录</p>
                    <p className="mt-2 text-sm leading-7 text-slate-500">
                      默认保存到当前目录。建议单独指定一个归档目录。
                    </p>
                  </div>
                  <button
                    onClick={chooseDirectory}
                    className="rounded-full border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:bg-slate-100"
                  >
                    选择目录
                  </button>
                </div>
                <div className="mt-4 rounded-[1.2rem] border border-dashed border-slate-300 bg-white px-4 py-4 text-sm text-slate-600">
                  {exportPath || '当前使用默认路径：.'}
                </div>
              </div>

              <div className="rounded-[1.5rem] border border-slate-200 bg-slate-50 p-5">
                <p className="text-sm font-semibold text-slate-700">导出格式</p>
                <div className="mt-4 grid gap-3 sm:grid-cols-2">
                  {exportFormats.map((format) => {
                    const selected = formats.has(format.id);
                    return (
                      <button
                        key={format.id}
                        onClick={() => toggleFormat(format.id)}
                        className={`rounded-[1.2rem] border px-4 py-4 text-left transition ${
                          selected
                            ? 'border-slate-900 bg-slate-900 text-white'
                            : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300'
                        }`}
                      >
                        <p className="text-sm font-bold">{format.label}</p>
                        <p className={`mt-2 text-xs leading-6 ${selected ? 'text-slate-300' : 'text-slate-500'}`}>
                          {format.description}
                        </p>
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>

            <div className="flex flex-col gap-4 rounded-[1.6rem] bg-slate-900 px-5 py-5 text-white lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p className="text-lg font-bold">
                  {syncMode === 'channel'
                    ? `准备同步 ${selectedChannels.size} 个频道`
                    : `准备处理 ${urlList.split('\n').map((line) => line.trim()).filter(Boolean).length} 条链接`}
                </p>
                <p className="mt-2 text-sm text-slate-300">
                  同步期间界面会持续记录状态，SQLite 会写入 `slackdump.db`。
                </p>
              </div>
              <button
                onClick={handleSync}
                disabled={isSyncing}
                className="rounded-[1.2rem] bg-amber-300 px-6 py-3 text-base font-bold text-slate-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:bg-slate-600 disabled:text-slate-300"
              >
                {isSyncing ? '同步中...' : '开始同步'}
              </button>
            </div>
          </section>

          <section className="rounded-[2rem] border border-white/70 bg-white/88 p-6 shadow-[0_24px_60px_rgba(127,92,46,0.09)] backdrop-blur">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold uppercase tracking-[0.24em] text-slate-400">运行日志</p>
                <h2 className="mt-2 text-2xl font-black text-slate-900">状态输出</h2>
              </div>
              <button
                onClick={() => setLogs([])}
                className="rounded-full bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
              >
                清空
              </button>
            </div>

            <div className="soft-scroll mt-5 max-h-[calc(100vh-14rem)] space-y-3 overflow-y-auto pr-1">
              {logs.length === 0 ? (
                <div className="rounded-[1.4rem] border border-dashed border-slate-300 px-4 py-6 text-sm leading-7 text-slate-500">
                  暂无日志输出。
                </div>
              ) : (
                logs.map((entry, index) => (
                  <div
                    key={`${entry}-${index}`}
                    className="rounded-[1.3rem] border border-slate-200 bg-slate-50 px-4 py-4 text-sm leading-7 text-slate-600"
                  >
                    {entry}
                  </div>
                ))
              )}
            </div>
          </section>
        </main>
      </div>
    </div>
  );
}

export default App;
