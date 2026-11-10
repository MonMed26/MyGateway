"use client";

import Link from "next/link";
import useSWR from "swr";
import { ArrowLeft, Copy, RefreshCw, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { AppShell } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, fetcher } from "@/lib/api";
import { formatDate, rupiah, statusClass } from "@/lib/format";
import type { TransactionDetailResponse } from "@/lib/types";
import { useQueryParam } from "@/lib/use-query";

export default function TransactionDetailPage() {
  return (
    <AppShell>
      <TransactionDetail />
    </AppShell>
  );
}

function TransactionDetail() {
  const id = useQueryParam("id");
  const { data, mutate, isLoading } = useSWR<TransactionDetailResponse>(
    id ? `/api/admin/transactions/${id}` : null,
    fetcher,
    { refreshInterval: 8000 },
  );

  async function resend() {
    if (!id) return;
    try {
      await api.post(`/api/admin/transactions/${id}/resend-webhook`);
      toast.success("Webhook dijadwalkan ulang");
      await mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal mengirim ulang webhook");
    }
  }

  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast.success("Disalin");
    } catch {
      toast.error("Gagal menyalin");
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (!data) {
    return <div className="text-sm text-muted-foreground">Transaksi tidak ditemukan.</div>;
  }

  const { payment, webhooks } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Button asChild variant="ghost" size="icon">
          <Link href="/transactions">
            <ArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Detail Transaksi</h1>
          <p className="font-mono text-xs text-muted-foreground">{payment.id}</p>
        </div>
        <span className={`ml-auto inline-flex items-center rounded-full border px-2.5 py-1 text-xs ${statusClass(payment.status)}`}>
          {payment.status}
        </span>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Ringkasan</CardTitle>
            <CardDescription>Nominal dan referensi payment intent.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-2">
            <Info label="Nominal dasar" value={rupiah(payment.base_amount)} />
            <Info label="Kode unik" value={`+${payment.unique_offset}`} />
            <Info label="Nominal ditransfer" value={rupiah(payment.unique_amount)} strong />
            <Info label="Reference" value={payment.reference || "—"} />
            <Info label="Dibuat" value={formatDate(payment.created_at)} />
            <Info label="Expired" value={formatDate(payment.expires_at)} />
            <Info label="Dibayar" value={formatDate(payment.paid_at)} />
            <Info label="Config ID" value={payment.config_id} />
            {payment.transaction && (
              <>
                <Info label="Status transaksi" value={payment.transaction.Status || "—"} />
                <Info label="ID transaksi" value={payment.transaction.ID || "—"} />
                <Info label="Tipe" value={payment.transaction.PaymentType || "—"} />
                <Info label="Waktu transaksi" value={formatDate(payment.transaction.Time)} />
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between">
              QRIS
              {payment.qr_string && (
                <Button variant="ghost" size="icon" onClick={() => copy(payment.qr_string!)} title="Salin">
                  <Copy className="h-4 w-4" />
                </Button>
              )}
            </CardTitle>
            <CardDescription>Payload dinamis yang dipindai pembeli.</CardDescription>
          </CardHeader>
          <CardContent>
            {payment.qr_string ? (
              <pre className="max-h-[260px] overflow-auto whitespace-pre-wrap break-all rounded-md border border-border/70 bg-background/40 p-3 text-[11px] leading-relaxed text-slate-300">
                {payment.qr_string}
              </pre>
            ) : (
              <div className="text-sm text-muted-foreground">Tidak ada QRIS (static QRIS belum diatur).</div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>Webhook</CardTitle>
            <CardDescription>Riwayat pengiriman notifikasi pembayaran.</CardDescription>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => mutate()}>
              <RefreshCw className="h-3.5 w-3.5" /> Refresh
            </Button>
            <Button variant="outline" size="sm" onClick={resend} disabled={payment.status !== "paid"}>
              <RotateCcw className="h-3.5 w-3.5" /> Kirim ulang
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {webhooks.length === 0 ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              Belum ada pengiriman webhook. Pastikan URL & secret sudah diatur di provider.
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Status</TableHead>
                  <TableHead>URL</TableHead>
                  <TableHead>Attempt</TableHead>
                  <TableHead>HTTP</TableHead>
                  <TableHead>Jadwal berikut</TableHead>
                  <TableHead>Dibuat</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {webhooks.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell>
                      <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs ${statusClass(d.status)}`}>
                        {d.status}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-[260px] truncate font-mono text-xs">{d.url}</TableCell>
                    <TableCell className="text-xs">{d.attempts}</TableCell>
                    <TableCell className="text-xs">{d.last_status_code || "—"}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {d.status === "pending" ? formatDate(d.next_attempt_at) : "—"}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{formatDate(d.created_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {payment.transaction && (
        <Card>
          <CardHeader>
            <CardTitle>Raw Transaction</CardTitle>
            <CardDescription>Payload transaksi dari feed provider.</CardDescription>
          </CardHeader>
          <CardContent>
            <pre className="max-h-[320px] overflow-auto rounded-md border border-border/70 bg-background/40 p-3 text-[11px] leading-relaxed text-slate-300">
              {JSON.stringify(payment.transaction, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function Info({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className="rounded-md border border-border/70 bg-background/40 px-3 py-2">
      <div className="text-[11px] uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className={`truncate text-sm ${strong ? "font-semibold text-primary" : ""}`}>{value}</div>
    </div>
  );
}
