"use client";

import Link from "next/link";
import { useState } from "react";
import useSWR from "swr";
import { ExternalLink, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { AppShell } from "@/components/app-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, fetcher } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { Provider, ProviderType } from "@/lib/types";

export default function ProvidersPage() {
  return (
    <AppShell>
      <Providers />
    </AppShell>
  );
}

function Providers() {
  const { data, mutate, isLoading } = useSWR<{ items: Provider[] }>("/api/admin/providers", fetcher);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [provider, setProvider] = useState<ProviderType>("shopee");
  const [busy, setBusy] = useState(false);

  async function createProvider(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const created = await api.post<Provider>("/api/admin/providers", { name, provider });
      toast.success("Provider dibuat");
      setOpen(false);
      setName("");
      setProvider("shopee");
      await mutate();
      window.location.href = `/providers/detail/?id=${created.id}`;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal membuat provider");
    } finally {
      setBusy(false);
    }
  }

  async function remove(p: Provider) {
    if (!window.confirm(`Hapus provider "${p.name}"? Transaksi lama tetap tersimpan.`)) return;
    try {
      await api.del(`/api/admin/providers/${p.id}`);
      toast.success("Provider dihapus");
      await mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal menghapus provider");
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Providers</h1>
          <p className="text-sm text-muted-foreground">
            Setiap provider punya API token, secret webhook, dan rentang kode unik sendiri.
          </p>
        </div>
        <Button onClick={() => setOpen(true)}>
          <Plus className="h-4 w-4" /> Tambah Provider
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          {isLoading || !data ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : data.items.length === 0 ? (
            <div className="flex flex-col items-center gap-3 py-16 text-center">
              <p className="text-sm text-muted-foreground">Belum ada provider terdaftar.</p>
              <Button onClick={() => setOpen(true)}>
                <Plus className="h-4 w-4" /> Tambah Provider
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Nama</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Merchant / Store</TableHead>
                  <TableHead>Poll interval</TableHead>
                  <TableHead>Diperbarui</TableHead>
                  <TableHead className="text-right">Aksi</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.items.map((p) => (
                  <TableRow key={p.id}>
                    <TableCell>
                      <div className="font-medium">{p.name}</div>
                      <div className="text-xs capitalize text-muted-foreground">
                        {p.provider} · {p.id}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <Badge variant={p.connected ? "default" : "outline"}>
                          {p.connected ? "Terhubung" : "Belum login"}
                        </Badge>
                        {!p.active && <span className="text-xs text-muted-foreground">nonaktif</span>}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      <div className="font-mono">{p.merchant_id || "—"}</div>
                      <div className="font-mono text-[11px]">{p.store_id || "—"}</div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{p.poll_interval_ms} ms</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{formatDate(p.updated_at)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button asChild variant="ghost" size="icon">
                          <Link href={`/providers/detail/?id=${p.id}`} title="Detail">
                            <ExternalLink className="h-4 w-4" />
                          </Link>
                        </Button>
                        <Button variant="ghost" size="icon" onClick={() => remove(p)} title="Hapus">
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <form onSubmit={createProvider}>
            <DialogHeader>
              <DialogTitle>Tambah Provider</DialogTitle>
              <DialogDescription>
                Pilih jenis provider. Setelah dibuat, sambungkan akun merchant lewat OTP.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="name">Nama</Label>
                <Input
                  id="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Toko Utama"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="provider">Provider</Label>
                <Select
                  id="provider"
                  value={provider}
                  onChange={(e) => setProvider(e.target.value as ProviderType)}
                >
                  <option value="shopee">Shopee</option>
                  <option value="gopay">GoPay / GoBiz</option>
                </Select>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Batal
              </Button>
              <Button type="submit" disabled={busy || !name}>
                {busy ? "Menyimpan…" : "Simpan"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
