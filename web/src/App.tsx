import { BrowserRouter, Routes, Route, Link, useLocation } from 'react-router-dom';
import Home from './routes/Home';
import Settings from './routes/Settings';

function NavBar() {
  const location = useLocation();

  return (
    <nav className="bg-white shadow-sm border-b">
      <div className="max-w-6xl mx-auto px-4">
        <div className="flex items-center h-14">
          <Link to="/" className="text-xl font-bold text-primary mr-8">
            AI 人际关系助手
          </Link>
          <div className="flex gap-4">
            <Link
              to="/"
              className={`px-3 py-2 rounded-lg transition-colors ${
                location.pathname === '/'
                  ? 'bg-primary/10 text-primary'
                  : 'text-gray-600 hover:bg-gray-100'
              }`}
            >
              首页
            </Link>
            <Link
              to="/settings"
              className={`px-3 py-2 rounded-lg transition-colors ${
                location.pathname === '/settings'
                  ? 'bg-primary/10 text-primary'
                  : 'text-gray-600 hover:bg-gray-100'
              }`}
            >
              设置
            </Link>
          </div>
        </div>
      </div>
    </nav>
  );
}

function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-surface">
        <NavBar />
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </div>
    </BrowserRouter>
  );
}

export default App;