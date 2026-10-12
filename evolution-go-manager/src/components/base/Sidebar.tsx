import { useEffect } from 'react';
import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Smartphone,
  X,
} from 'lucide-react';
import { cn } from '@/utils/cn';

const navItems = [
  { to: '/manager', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/manager/instances', label: 'Instâncias', icon: Smartphone },
];

interface SidebarProps {
  // Gaveta do mobile: abaixo de md a sidebar fixa some e só aparece por aqui.
  mobileOpen?: boolean;
  onMobileClose?: () => void;
}

function SidebarContent({ onNavigate, onClose }: { onNavigate?: () => void; onClose?: () => void }) {
  const currentYear = new Date().getFullYear();

  return (
    <>
      {/* Logo Header */}
      <div className="h-16 flex items-center justify-between px-4 border-b border-sidebar-border">
        <h2 className="text-lg font-bold text-primary">
          Evolution GO
        </h2>
        {onClose && (
          <button
            onClick={onClose}
            className="rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label="Fechar menu"
          >
            <X className="h-5 w-5" />
          </button>
        )}
      </div>

      {/* Navigation Menu */}
      <nav className="space-y-1.5 flex-1 px-2 py-4">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/manager'}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 px-3 py-2.5 rounded-md transition-all',
                isActive
                  ? 'bg-primary/10 text-primary'
                  : 'text-muted-foreground hover:text-foreground hover:bg-accent'
              )
            }
          >
            {({ isActive }) => (
              <>
                <item.icon className={cn('flex-shrink-0 h-5 w-5', isActive && 'text-primary')} />
                <span className="font-medium">{item.label}</span>
              </>
            )}
          </NavLink>
        ))}
      </nav>

      {/* Sidebar Footer */}
      <div className="mt-auto p-4 border-t border-sidebar-border">
        <div className="text-sm text-primary font-medium">Evolution GO</div>
        <div className="text-xs text-muted-foreground mt-1">
          © {currentYear} All rights reserved
        </div>
      </div>
    </>
  );
}

function Sidebar({ mobileOpen = false, onMobileClose }: SidebarProps) {
  useEffect(() => {
    if (!mobileOpen || !onMobileClose) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onMobileClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [mobileOpen, onMobileClose]);

  return (
    <>
      <div className="hidden md:flex bg-sidebar text-sidebar-foreground flex-col w-56 border-r border-sidebar-border">
        <SidebarContent />
      </div>

      {mobileOpen && (
        <div className="fixed inset-0 z-50 flex md:hidden" role="dialog" aria-modal="true">
          <div className="absolute inset-0 bg-black/50" onClick={onMobileClose} />
          <div className="relative flex w-64 max-w-[80%] flex-col bg-sidebar text-sidebar-foreground border-r border-sidebar-border">
            <SidebarContent onNavigate={onMobileClose} onClose={onMobileClose} />
          </div>
        </div>
      )}
    </>
  );
}

export default Sidebar;
