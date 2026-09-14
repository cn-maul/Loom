import { useEffect, useState } from 'react';
import { BrowserRouter, Link, Navigate, NavLink, Route, Routes, useLocation } from 'react-router-dom';
import {
  Building2,
  ChevronsLeft,
  ChevronsRight,
  FileText,
  Home,
  MessageSquareText,
  Moon,
  NotebookPen,
  Search,
  Settings,
  Sun,
  Users,
} from 'lucide-react';
import Advice from './routes/Advice';
import EventDetail from './routes/EventDetail';
import Events from './routes/Events';
import HomePage from './routes/Home';
import Organizations from './routes/Organizations';
import PersonDetail from './routes/PersonDetail';
import Report from './routes/Report';
import SettingsPage from './routes/Settings';
import { QUICK_RECORD_EVENT, UNAUTHORIZED_EVENT, authToken } from './api/client';
import CommandPalette from './components/CommandPalette';
import QuickRecordModal from './components/QuickRecordModal';
import { Button } from './components/ui/button';
import { controlClass } from './components/ui';
import { cn } from './lib/utils';
import { useTheme } from './lib/theme';

type NavEntry = { to: string; icon: typeof Home; label: string; end?: boolean };

const NAV_GROUPS: { label: string; items: NavEntry[] }[] = [
  {
    label: '日常',
    items: [
      { to: '/', icon: Home, label: '首页', end: true },
      { to: '/events', icon: NotebookPen, label: '记录' },
      { to: '/advice', icon: MessageSquareText, label: '问一问' },
    ],
  },
  {
    label: '关系',
    items: [{ to: '/organizations', icon: Building2, label: '人物与组织' }],
  },
  {
    label: '回顾',
    items: [{ to: '/report', icon: FileText, label: '周报' }],
  },
  {
    label: '系统',
    items: [{ to: '/settings', icon: Settings, label: '设置' }],
  },
];

function NavItem({ to, icon: Icon, label, collapsed, end = false }: NavEntry & { collapsed: boolean }) {
  return (
    <NavLink
      to={to}
      end={end}
      title={collapsed ? label : undefined}
      className={({ isActive }) =>
        cn(
          'group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
          collapsed && 'justify-center px-2',
          isActive
            ? 'bg-primary/10 font-medium text-primary'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground',
        )
      }
    >
      {({ isActive }) => (
        <>
          {/* Active marker: a short bar that reads as "you are here" even when collapsed. */}
          {isActive ? (
            <span className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-r bg-primary" />
          ) : null}
          <Icon className="size-4 shrink-0" />
          {!collapsed && <span className="truncate">{label}</span>}
        </>
      )}
    </NavLink>
  );
}

function Shell() {
  const [collapsed, setCollapsed] = useState(false);
  const [recordOpen, setRecordOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const { dark, toggle } = useTheme();
  const location = useLocation();

  // Any page can open the global "记一笔" drawer by dispatching this event,
  // so the record button can live where it matters (the landing page).
  useEffect(() => {
    const onQuickRecord = () => setRecordOpen(true);
    window.addEventListener(QUICK_RECORD_EVENT, onQuickRecord);
    return () => window.removeEventListener(QUICK_RECORD_EVENT, onQuickRecord);
  }, []);

  // Ctrl/⌘+K opens the person palette from anywhere — the app-wide shortcut
  // for "I need to pull up someone right now".
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setSearchOpen((open) => !open);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  return (
    <div className="flex h-screen overflow-hidden bg-muted/30">
      <aside
        className={cn(
          'flex shrink-0 flex-col border-r border-sidebar-border bg-sidebar transition-[width] duration-200',
          collapsed ? 'w-16' : 'w-56',
        )}
      >
        <div className={cn('flex h-14 items-center gap-2 border-b border-sidebar-border px-3', collapsed && 'justify-center px-2')}>
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
            <Users className="size-4" />
          </div>
          {!collapsed && (
            <Link to="/" className="truncate text-[15px] font-semibold tracking-tight text-sidebar-foreground">
              Loom
            </Link>
          )}
        </div>

        <nav className="scroll-slim flex-1 overflow-y-auto px-2 pb-4 pt-3">
          {NAV_GROUPS.map((group) => (
            <div key={group.label} className="mb-3">
              {!collapsed ? (
                <p className="px-3 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground/70">
                  {group.label}
                </p>
              ) : (
                <div className="mx-2 mb-1 border-t border-sidebar-border" />
              )}
              <div className="space-y-0.5">
                {group.items.map((item) => (
                  <NavItem key={item.to} {...item} collapsed={collapsed} />
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="flex items-center justify-between border-t border-sidebar-border p-2">
          {!collapsed && <span className="pl-1 text-[11px] text-muted-foreground">个人关系管理</span>}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setCollapsed((c) => !c)}
            aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
            title={collapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {collapsed ? <ChevronsRight className="size-4" /> : <ChevronsLeft className="size-4" />}
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b bg-background/80 px-4 backdrop-blur">
          {/* Person lookup is the other app-wide verb; the empty header half
              used to be dead space, now it is the search entry. */}
          <button
            type="button"
            onClick={() => setSearchOpen(true)}
            className="flex h-9 w-full max-w-sm items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 text-sm text-muted-foreground transition-colors hover:border-primary/50 hover:text-foreground"
            title="搜索人物（Ctrl+K）"
          >
            <Search className="size-3.5 shrink-0" />
            <span className="truncate">搜索人物…</span>
            <kbd className="ml-auto hidden rounded border border-border bg-background px-1.5 py-0.5 text-[11px] sm:inline">Ctrl K</kbd>
          </button>

          <div className="ml-auto flex items-center gap-2">
            <Button variant="ghost" size="icon" onClick={toggle} aria-label="切换主题" title="切换主题">
              {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </Button>
          </div>
        </header>

        <main className="scroll-slim flex-1 overflow-y-auto p-4 md:p-6">
          {/* Remount on pathname so a page never keeps the previous route's scroll position. */}
          <div key={location.pathname} className="mx-auto w-full max-w-[1400px]">
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/persons/:id" element={<PersonDetail />} />
              <Route path="/events" element={<Events />} />
              <Route path="/events/:id" element={<EventDetail />} />
              <Route path="/organizations" element={<Organizations />} />
              <Route path="/advice" element={<Advice />} />
              <Route path="/report" element={<Report />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </div>
        </main>
      </div>

      <QuickRecordModal open={recordOpen} onClose={() => setRecordOpen(false)} />
      <CommandPalette open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}

// TokenGate listens for 401s from the API layer and asks for the access token
// once. Saving reloads the page, so every in-flight view refetches with the
// token attached instead of patching stale state.
function TokenGate() {
  const [open, setOpen] = useState(false);
  const [token, setToken] = useState('');
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const onUnauthorized = () => {
      setFailed(true);
      setOpen(true);
    };
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
  }, []);

  if (!open) return null;

  const submit = () => {
    if (!token.trim()) return;
    authToken.set(token.trim());
    window.location.reload();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur">
      <div className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-lg">
        <h2 className="text-base font-semibold text-foreground">需要访问令牌</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {failed ? '令牌无效或未填写，请求被服务器拒绝。' : '此 Loom 实例开启了访问控制。'}
          令牌由 config.yaml 的 <code className="rounded bg-muted px-1 text-xs">server.auth_token</code> 配置，只保存在本浏览器。
        </p>
        <input
          type="password"
          autoFocus
          value={token}
          onChange={(e) => setToken(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          placeholder="访问令牌"
          className={`${controlClass} mt-4`}
        />
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="outline" onClick={() => setOpen(false)}>
            稍后再说
          </Button>
          <Button onClick={submit} disabled={!token.trim()}>
            保存并刷新
          </Button>
        </div>
      </div>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Shell />
      <TokenGate />
    </BrowserRouter>
  );
}
