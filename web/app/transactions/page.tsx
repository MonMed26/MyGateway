"use client";

import Link from "next/link";
import { useState } from "react";
import useSWR from "swr";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { fetcher } from "@/lib/api";
import { formatDate, rupiah, shortId, statusClass } from "@/lib/format";
import type { Provider, TransactionsResponse } from "@/lib/types";

export default function TransactionsPage() {
  return (
    <AppShell>
      <Transactions />
    </AppShell>
  );
}

function Transactions() {
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("");
  const [configId, setConfigId] = useState("");
  const [q, setQ] = useState("");
  const pageSize = 20;

  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (status) params.set("status", status);
  if (configId) params.set("config_id", configId);
  if (q.trim()) params.set("q", q.trim());

  const { data, isLoading } = useSWR<TransactionsResponse>(
    `/api/admin/transactions?${params.toString()}`,
    fetcher,
    { refreshInterval: 10000 },
  );
  const { data: providers } = useSWR<{ items: Provider[] }>("/api/admin/providers", fetcher);

  const configName = new Map(providers?.items.map((p) => [p.id, `${p.name} (${p.provider})`]) ?? []);
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));

  function reset() {
    setPage(1);
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Transaksi</h1>
        <p className="text-sm text-muted-foreground">Semua payment intent, status, dan riwayat webhook.</p>
      </div>

      <Card>
        <CardContent className="flex flex-col gap-3 p-4 md:flex-row md:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={q}
              onChange={(e) => {
                setQ(e.target.value);
                reset();
              }}
              placeholder="Cari ID atau reference…"
              className="pl-9"
            />
          </div>
          <Select
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              reset();
            }}
            className="md:w-40"
          >
            <option value="">Semua status</option>
            <option value="pending">Pending</option>
            <option value="paid">Paid</option>
            <option value="expired">Expired</option>
            <option value="cancelled">Cancelled</option>
          </Select>
          <Select
            value={configId}
            onChange={(e) => {
              setConfigId(e.target.value);
              reset();
            }}
            className="md:w-56"
          >
            <option value="">Semua provider</option>
            {providers?.items.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.provider})
              </option>
            ))}
          </Select>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : !data || data.items.length === 0 ? (
            <div className="py-16 text-center text-sm text-muted-foreground">Tidak ada transaksi yang cocok.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID / Reference</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>Nominal</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Dibuat</TableHead>
                  <TableHead className="text-right">Aksi</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.items.map((p) => (
                  <TableRow key={p.id}>
                    <TableCell>
                      <Link href={`/transactions/detail/?id=${p.id}`} className="font-mono text-xs hover:text-primary">
                        {shortId(p.id)}
                      </Link>
                      <div className="max-w-[220px] truncate text-xs text-muted-foreground">{p.reference || "—"}</div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {configName.get(p.config_id) || p.config_id}
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{rupiah(p.unique_amount)}</div>
                      <div className="text-xs text-muted-foreground">base {rupiah(p.base_amount)}</div>
                    </TableCell>
                    <TableCell>
                      <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs ${statusClass(p.status)}`}>
                        {p.status}
                      </span>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{formatDate(p.created_at)}</TableCell>
                    <TableCell className="text-right">
                      <Button asChild variant="outline" size="sm">
                        <Link href={`/transactions/detail/?id=${p.id}`}>Detail</Link>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <div>
          {total} transaksi · halaman {page} dari {pages}
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            <ChevronLeft className="h-4 w-4" /> Sebelumnya
          </Button>
          <Button variant="outline" size="sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            Berikutnya <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
