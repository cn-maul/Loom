import { useState } from 'react';
import { BrowserRouter, Link, Navigate, NavLink, Route, Routes, useLocation } from 'react-router-dom';
import {
  Building2,
  ChevronsLeft,
  ChevronsRight,
  FileText,
  Home,
  ListTodo,
  MessageSquareText,
  Moon,
  NotebookPen,
  Settings,
  Share2,
  Sun,
  Users,
} from 'lucide-react';
import Advice from './routes/Advice';
import EventDetail from './routes/EventDetail';
import Events from './routes/Events';
import FollowUps from './routes/FollowUps';
import HomePage from './routes/Home';
import Organizations from './routes/Organizations';
import PersonDetail from './routes/PersonDetail';
import Relationships from './routes/Relationships';
import Report from './routes/Report';
import SettingsPage from './routes/Settings';
import { Button, buttonVariants } from './components/ui/button';
import { cn } from './lib/utils';
import { useTheme } from './lib/theme';

type NavEntry = { to: string; icon: typeof Home; label: string; end?: boolean };

const NAV_GROUPS: { label: string; items: NavEntry[] }[] = [
  {
    label: '日常',
    items: [
      { to: '/', icon: Home, label: '首页', end: true },
      { to: '/events', icon: NotebookPen, label: '记录' },
      { to: '/follow-ups', icon: ListTodo, label: '跟进' },
    ],
  },
  {
    label: '关系',
    items: [
      { to: '/organizations', icon: Building2, label: '组织' },
      { to: '/relationships', icon: Share2, label: '关系图谱' },
    ],
  },
  {
    label: '洞察',
    items: [
      { to: '/advice', icon: MessageSquareText, label: '建议' },
      { to: '/report', icon: FileText, label: '周报' },
    ],
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
  const { dark, toggle } = useTheme();
  const location = useLocation();

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
        <header className="flex h-14 shrink-0 items-center justify-end gap-2 border-b bg-background/80 px-4 backdrop-blur">
          {/* Writing a record is the one action worth reaching from anywhere. */}
          <Link to="/" className={buttonVariants({ variant: 'outline', size: 'sm' })} title="记一笔">
            <NotebookPen className="size-3.5" />
            记一笔
          </Link>
          <Button variant="ghost" size="icon" onClick={toggle} aria-label="切换主题" title="切换主题">
            {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </Button>
        </header>

        <main className="scroll-slim flex-1 overflow-y-auto p-4 md:p-6">
          {/* Remount on pathname so a page never keeps the previous route's scroll position. */}
          <div key={location.pathname} className="mx-auto w-full max-w-[1400px]">
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/persons/:id" element={<PersonDetail />} />
              <Route path="/events" element={<Events />} />
              <Route path="/events/:id" element={<EventDetail />} />
              <Route path="/follow-ups" element={<FollowUps />} />
              <Route path="/organizations" element={<Organizations />} />
              <Route path="/relationships" element={<Relationships />} />
              <Route path="/advice" element={<Advice />} />
              <Route path="/report" element={<Report />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </div>
        </main>
      </div>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Shell />
    </BrowserRouter>
  );
}
