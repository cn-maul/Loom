import { useEffect, useRef, useState } from 'react';
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
import { useEnterState } from './lib/overlay';

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

/**
 * One sidebar row. The active state is a quiet fill — no coloured left bar,
 * no tinted icon: "you are here" is carried by weight and a slightly darker
 * plate, which is how the system sidebars do it.
 */
function NavItem({ to, icon: Icon, label, collapsed, end = false }: NavEntry & { collapsed: boolean }) {
  return (
    <NavLink
      to={to}
      end={end}
      title={label}
      className={({ isActive }) =>
        cn(
          'group relative flex h-10 items-center gap-2.5 rounded-md px-2.5 text-[13.5px] transition-colors max-md:h-11',
          collapsed && 'justify-center px-0',
          isActive
            ? 'bg-fill font-semibold text-foreground'
            : 'text-ink-2 hover:bg-fill/60 hover:text-foreground',
        )
      }
    >
      <Icon className="size-[17px] shrink-0" />
      <span className={cn('truncate', collapsed && 'hidden', 'max-md:hidden')}>{label}</span>
    </NavLink>
  );
}

function Shell() {
  const [collapsed, setCollapsed] = useState(false);
  const [recordOpen, setRecordOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  // The hairline under the sticky bar only appears once content has scrolled
  // beneath it — a permanent divider on a flush bar reads as a seam.
  const [scrolled, setScrolled] = useState(false);
  const { dark, toggle } = useTheme();
  const location = useLocation();
  const mainRef = useRef<HTMLElement>(null);

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
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      {/* Structural region: a shade under the ground, separated by a hairline.
          Never glass — nothing overlaps it. Below 680px it becomes an icon
          rail rather than disappearing, so navigation stays one tap away. */}
      <aside
        className={cn(
          'flex shrink-0 flex-col border-r border-hairline bg-sidebar transition-[width] duration-200 max-md:w-16',
          collapsed ? 'w-16' : 'w-60',
        )}
      >
        <div
          className={cn(
            'flex h-14 items-center gap-2.5 px-3.5 max-md:justify-center max-md:px-0',
            collapsed && 'justify-center px-0',
          )}
        >
          <Link
            to="/"
            className="flex size-8 shrink-0 items-center justify-center rounded-[6px] bg-foreground text-background"
            aria-label="Loom 首页"
          >
            <Users className="size-[17px]" />
          </Link>
          <Link
            to="/"
            className={cn(
              'truncate text-[15px] font-semibold tracking-[-0.02em] text-sidebar-foreground',
              collapsed && 'hidden',
              'max-md:hidden',
            )}
          >
            Loom
          </Link>
        </div>

        <nav className="scroll-slim flex-1 overflow-y-auto px-2.5 pb-4 pt-2">
          {NAV_GROUPS.map((group) => (
            <div key={group.label} className="mb-4">
              <p
                className={cn(
                  'px-2.5 pb-1.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-ink-3',
                  collapsed && 'hidden',
                  'max-md:hidden',
                )}
              >
                {group.label}
              </p>
              {collapsed ? <div className="mx-2 mb-2 border-t border-hairline max-md:hidden" /> : null}
              <div className="space-y-0.5">
                {group.items.map((item) => (
                  <NavItem key={item.to} {...item} collapsed={collapsed} />
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="flex items-center justify-between p-2.5">
          <span
            className={cn('pl-1 text-[11px] text-ink-3', collapsed && 'hidden', 'max-md:hidden')}
          >
            个人关系管理
          </span>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setCollapsed((c) => !c)}
            aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
            title={collapsed ? '展开侧边栏' : '收起侧边栏'}
            className={cn('size-9 max-md:hidden', collapsed && 'mx-auto')}
          >
            {collapsed ? <ChevronsRight className="size-4" /> : <ChevronsLeft className="size-4" />}
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        {/* The one place glass is mandatory: a bar that scrolls content under it. */}
        <header
          className="glass-nav flex h-14 shrink-0 items-center gap-3 px-4 md:px-6"
          data-scrolled={scrolled}
        >
          {/* Person lookup is the other app-wide verb. */}
          <button
            type="button"
            onClick={() => setSearchOpen(true)}
            className="flex h-9 w-full max-w-sm items-center gap-2 rounded-full bg-fill px-3.5 text-[13px] text-ink-3 transition-colors hover:text-foreground"
            title="搜索人物（Ctrl+K）"
          >
            <Search className="size-3.5 shrink-0" />
            <span className="truncate">搜索人物</span>
            <kbd className="ml-auto hidden rounded-[6px] border border-hairline bg-card/70 px-1.5 py-0.5 text-[10.5px] sm:inline">
              Ctrl K
            </kbd>
          </button>

          <div className="ml-auto flex items-center gap-1">
            <Button variant="ghost" size="icon" onClick={toggle} aria-label="切换主题" title="切换主题">
              {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </Button>
          </div>
        </header>

        <main
          ref={mainRef}
          className="scroll-slim flex-1 overflow-y-auto"
          onScroll={(e) => setScrolled(e.currentTarget.scrollTop > 8)}
        >
          {/* Remount on pathname so a page never keeps the previous route's scroll position. */}
          <div key={location.pathname} className="mx-auto w-full max-w-[1080px] px-5 py-8 md:px-8 md:py-12">
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
  // This layer mounts already-open, so it needs the frame to transition from.
  const state = useEnterState(open);

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
    <div
      data-state={state}
      className="al-scrim fixed inset-0 z-50 flex items-center justify-center p-5"
      role="dialog"
      aria-modal="true"
    >
      <div className="al-sheet w-full max-w-sm rounded-2xl bg-card p-[22px] shadow-overlay">
        <h2 className="text-[17px] font-semibold tracking-[-0.02em] text-foreground">需要访问令牌</h2>
        <p className="mt-2 text-[13px] leading-[1.65] text-muted-foreground">
          {failed ? '令牌无效或未填写，请求被服务器拒绝。' : '此 Loom 实例开启了访问控制。'}
          令牌由 config.yaml 的{' '}
          <code className="rounded-[6px] bg-fill px-1 py-0.5 font-mono text-[12px]">server.auth_token</code>{' '}
          配置，只保存在本浏览器。
        </p>
        <input
          type="password"
          autoFocus
          value={token}
          onChange={(e) => setToken(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          placeholder="访问令牌"
          className={`${controlClass} mt-5`}
        />
        <div className="mt-5 flex justify-end gap-2">
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
