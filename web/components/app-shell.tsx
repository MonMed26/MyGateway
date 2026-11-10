"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import useSWR from "swr";
import { LayoutDashboard, LogOut, PlugZap, ReceiptText, Settings, Wallet } from "lucide-react";
import { api, fetcher } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

const nav = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard },
  { href: "/providers", label: "Providers", icon: PlugZap },
  { href: "/transactions", label: "Transaksi", icon: ReceiptText },
  { href: "/settings", label: "Pengaturan", icon: Settings },
];

function isActive(pathname: string, href: string) {
  if (href === "/") return pathname === "/";
  return pathname.startsWith(href);
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { data, error, isLoading } = useSWR<{ username: string }>("/api/admin/me", fetcher);
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (error && (error as { status?: number }).status === 401) {
      router.replace("/login");
    }
  }, [error, router]);

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        Memuat dashboard…
      </div>
    );
  }

  if (error && (error as { status?: number }).status === 401) {
    return null;
  }

  async function logout() {
    try {
      await api.post("/api/admin/logout");
    } catch {
      /* ignore */
    }
    router.replace("/login");
  }

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-border/70 bg-card/40 px-4 py-6 md:flex">
        <Link href="/" className="mb-8 flex items-center gap-2 px-2">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Wallet className="h-5 w-5" />
          </span>
          <div>
            <div className="text-sm font-semibold tracking-tight">MyGateway</div>
            <div className="text-[11px] uppercase tracking-widest text-muted-foreground">Gateway</div>
          </div>
        </Link>

        <nav className="flex flex-1 flex-col gap-1">
          {nav.map((item) => {
            const Icon = item.icon;
            const active = isActive(pathname, item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors",
                  active
                    ? "bg-primary/10 text-primary"
                    : "text-muted-foreground hover:bg-white/5 hover:text-foreground",
                )}
              >
                <Icon className="h-4 w-4" />
                {item.label}
              </Link>
            );
          })}
        </nav>

        <div className="mt-6 rounded-lg border border-border/70 bg-background/40 p-3">
          <div className="text-[11px] uppercase tracking-widest text-muted-foreground">Masuk sebagai</div>
          <div className="mt-1 truncate text-sm font-medium">{data?.username ?? "admin"}</div>
          <Button variant="outline" size="sm" className="mt-3 w-full" onClick={logout}>
            <LogOut className="h-3.5 w-3.5" /> Keluar
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-border/70 bg-card/30 px-4 py-3 md:hidden">
          <Link href="/" className="flex items-center gap-2 font-semibold">
            <Wallet className="h-5 w-5 text-primary" /> MyGateway
          </Link>
          <div className="flex items-center gap-1">
            {nav.map((item) => {
              const Icon = item.icon;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "rounded-md p-2",
                    isActive(pathname, item.href) ? "bg-primary/10 text-primary" : "text-muted-foreground",
                  )}
                >
                  <Icon className="h-4 w-4" />
                </Link>
              );
            })}
            <Button variant="ghost" size="icon" onClick={logout}>
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </header>

        <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 md:px-8 md:py-10">{children}</main>
      </div>
    </div>
  );
}
