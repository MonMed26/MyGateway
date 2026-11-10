"use client";

import Link from "next/link";
import useSWR from "swr";
import { ArrowRight, CheckCircle2, Clock, PlugZap, ReceiptText, TrendingUp, Wallet } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { fetcher } from "@/lib/api";
import { formatDate, rupiah, shortId, statusClass } from "@/lib/format";
import type { Provider, Stats, TransactionsResponse } from "@/lib/types";

export default function DashboardPage() {
  return (
    <AppShell>
      <Dashboard />
    </AppShell>
  );
}

function Dashboard() {
  const { data: stats } = useSWR<Stats>("/api/admin/stats", fetcher, { refreshInterval: 10000 });
  const { data: tx } = useSWR<TransactionsResponse>("/api/admin/transactions?page_size=8", fetcher, {
    refreshInterval: 10000,
  });
  const { data: providers } = useSWR<{ items: Provider[] }>("/api/admin/providers", fetcher, {
    refreshInterval: 15000,
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
          <p className="text-sm text-muted-foreground">Ringkasan pembayaran dan status provider.</p>
        </div>
        <div className="flex gap-2">
          <Button asChild variant="outline">
            <Link href="/transactions">
              <ReceiptText className="h-4 w-4" /> Transaksi
            </Link>
          </Button>
          <Button asChild>
            <Link href="/providers">
              <PlugZap className="h-4 w-4" /> Kelola Provider
            </Link>
          </Button>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Total Transaksi"
          value={stats?.total_payments}
          icon={<ReceiptText className="h-4 w-4" />}
          hint={`${stats?.pending ?? 0} pending · ${stats?.expired ?? 0} expired`}
        />
        <StatCard
          title="Lunas"
          value={stats?.paid_payments}
          icon={<CheckCircle2 className="h-4 w-4" />}
          hint="Pembayaran berhasil"
        />
        <StatCard
          title="Volume Lunas"
          value={stats ? rupiah(stats.paid_volume) : undefined}
          icon={<TrendingUp className="h-4 w-4" />}
          hint="Total nominal unik yang terbayar"
        />
        <StatCard
          title="Provider"
          value={stats?.providers}
          icon={<PlugZap className="h-4 w-4" />}
          hint={`${stats?.connected ?? 0} terhubung`}
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle>Transaksi Terbaru</CardTitle>
              <CardDescription>8 transaksi terakhir.</CardDescription>
            </div>
            <Button asChild variant="ghost" size="sm">
              <Link href="/transactions">
                Semua <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            </Button>
          </CardHeader>
          <CardContent>
            {!tx ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : tx.items.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-10 text-center text-sm text-muted-foreground">
                <Wallet className="mb-2 h-6 w-6" />
                Belum ada transaksi. Buat payment lewat API untuk memulai.
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>Nominal</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Dibuat</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tx.items.map((p) => (
                    <TableRow key={p.id}>
                      <TableCell>
                        <Link href={`/transactions/detail/?id=${p.id}`} className="font-mono text-xs hover:text-primary">
                          {shortId(p.id)}
                        </Link>
                        <div className="text-xs text-muted-foreground">{p.reference || "—"}</div>
                      </TableCell>
                      <TableCell className="font-medium">{rupiah(p.unique_amount)}</TableCell>
                      <TableCell>
                        <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs ${statusClass(p.status)}`}>
                          {p.status}
                        </span>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{formatDate(p.created_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Providers</CardTitle>
            <CardDescription>Status koneksi merchant.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {!providers ? (
              <Skeleton className="h-24 w-full" />
            ) : providers.items.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border p-4 text-center text-sm text-muted-foreground">
                Belum ada provider.
                <Button asChild size="sm" className="mt-3 w-full">
                  <Link href="/providers">Tambah Provider</Link>
                </Button>
              </div>
            ) : (
              providers.items.map((p) => (
                <Link
                  key={p.id}
                  href={`/providers/detail/?id=${p.id}`}
                  className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2.5 transition-colors hover:border-primary/40 hover:bg-white/[0.03]"
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{p.name}</div>
                    <div className="text-xs capitalize text-muted-foreground">{p.provider}</div>
                  </div>
                  <Badge variant={p.connected ? "default" : "outline"}>
                    {p.connected ? "Terhubung" : "Belum"}
                  </Badge>
                </Link>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function StatCard({
  title,
  value,
  hint,
  icon,
}: {
  title: string;
  value?: number | string;
  hint: string;
  icon: React.ReactNode;
}) {
  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex items-center justify-between">
          <span className="text-xs uppercase tracking-widest text-muted-foreground">{title}</span>
          <span className="rounded-md bg-primary/10 p-1.5 text-primary">{icon}</span>
        </div>
        <div className="mt-3 text-2xl font-semibold tracking-tight">
          {value === undefined ? <Skeleton className="h-8 w-20" /> : value}
        </div>
        <div className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
          <Clock className="h-3 w-3" /> {hint}
        </div>
      </CardContent>
    </Card>
  );
}
