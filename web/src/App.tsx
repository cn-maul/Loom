import { useState } from 'react';
import { BrowserRouter, Link, Navigate, NavLink, Route, Routes } from 'react-router-dom';
import { FileText, Home, MessageSquareText, Moon, PanelLeft, Settings, Sun, Users } from 'lucide-react';
import Advice from './routes/Advice';
import EventDetail from './routes/EventDetail';
import HomePage from './routes/Home';
import PersonDetail from './routes/PersonDetail';
import Report from './routes/Report';
import SettingsPage from './routes/Settings';
import { Button } from './components/ui/button';
import { cn } from './lib/utils';
import { useTheme } from './lib/theme';

const NAV = [
  { to: '/', icon: Home, label: '首页', end: true },
  { to: '/advice', icon: MessageSquareText, label: '建议' },
  { to: '/report', icon: FileText, label: '周报' },
  { to: '/settings', icon: Settings, label: '设置' },
];

function NavItem({ to, icon: Icon, label, collapsed, end = false }: { to: string; icon: typeof Home; label: string; collapsed: boolean; end?: boolean }) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          collapsed && 'justify-center px-2',
          isActive ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
        )
      }
    >
      <Icon className="size-4 shrink-0" />
      {!collapsed && <span>{label}</span>}
    </NavLink>
  );
}

function Shell() {
  const [collapsed, setCollapsed] = useState(false);
  const { dark, toggle } = useTheme();

  return (
    <div className="flex h-screen overflow-hidden bg-muted/30">
      <aside
        className={cn(
          'flex shrink-0 flex-col border-r bg-sidebar transition-[width] duration-200',
          collapsed ? 'w-16' : 'w-56',
        )}
      >
        <div className="flex h-14 items-center gap-2 border-b px-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Users className="size-4" />
          </div>
          {!collapsed && (
            <Link to="/" className="text-base font-semibold text-sidebar-foreground">
              Loom
            </Link>
          )}
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto px-2 pb-4 pt-2">
          {NAV.map((item) => (
            <NavItem key={item.to} {...item} collapsed={collapsed} />
          ))}
        </nav>

        <div className="border-t p-2">
          <Button variant="ghost" size="icon" onClick={() => setCollapsed((c) => !c)} aria-label="收起侧边栏">
            <PanelLeft className="size-4" />
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center justify-end gap-1.5 border-b bg-background px-4">
          <Button variant="ghost" size="icon" onClick={toggle} aria-label="切换主题">
            {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </Button>
        </header>

        <main className="flex-1 overflow-y-auto p-4 md:p-6">
          <div className="mx-auto w-full max-w-[1600px]">
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/persons/:id" element={<PersonDetail />} />
              <Route path="/events/:id" element={<EventDetail />} />
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
