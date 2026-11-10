"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import { Copy, KeyRound, RefreshCw, Save, ShieldCheck, Webhook } from "lucide-react";
import { toast } from "sonner";
import { AppShell } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { api, fetcher } from "@/lib/api";
import { maskSecret } from "@/lib/format";
import type { GlobalSettings, Provider } from "@/lib/types";

export default function SettingsPage() {
  return (
    <AppShell>
      <SettingsView />
    </AppShell>
  );
}

function SettingsView() {
  const { data, mutate, isLoading } = useSWR<GlobalSettings>("/api/admin/settings", fetcher);
  const { data: providers } = useSWR<{ items: Provider[] }>("/api/admin/providers", fetcher);

  const [form, setForm] = useState({
    webhook_url: "",
    webhook_secret: "",
    unique_min: 1,
    unique_max: 999,
    default_provider_id: "",
  });
  const [busy, setBusy] = useState(false);
  const [showToken, setShowToken] = useState(false);

  useEffect(() => {
    if (data) {
      setForm({
        webhook_url: data.webhook_url ?? "",
        webhook_secret: data.webhook_secret ?? "",
        unique_min: data.unique_min,
        unique_max: data.unique_max,
        default_provider_id: data.default_provider_id ?? "",
      });
    }
  }, [data]);

  async function save() {
    setBusy(true);
    try {
      await api.patch("/api/admin/settings", form);
      await mutate();
      toast.success("Pengaturan global disimpan");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal menyimpan pengaturan");
    } finally {
      setBusy(false);
    }
  }

  async function rotateToken() {
    if (!window.confirm("Rotate API token global? Semua klien harus memakai token baru.")) return;
    try {
      await api.post("/api/admin/settings/rotate-api-token");
      await mutate();
      toast.success("API token global dirotasi");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal merotasi token");
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

  if (isLoading || !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const apiToken = data.api_token;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Pengaturan Global</h1>
        <p className="text-sm text-muted-foreground">
          API key, webhook, dan unique code berlaku untuk semua provider dan semua transaksi.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="h-4 w-4 text-primary" /> API Key
            </CardTitle>
            <CardDescription>Satu Bearer token untuk semua request klien ke gateway.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-2">
              <Label>API Token (Bearer)</Label>
              <div className="flex gap-2">
                <Input
                  readOnly
                  value={showToken ? apiToken : maskSecret(apiToken)}
                  className="font-mono text-xs"
                />
                <Button variant="outline" size="icon" onClick={() => setShowToken((v) => !v)} title="Tampilkan">
                  <ShieldCheck className="h-4 w-4" />
                </Button>
                <Button variant="outline" size="icon" onClick={() => copy(apiToken)} title="Salin">
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </div>
            <Button variant="ghost" size="sm" onClick={rotateToken}>
              <RefreshCw className="mr-1 h-3.5 w-3.5" /> Rotate token
            </Button>

            <Separator />

            <div className="rounded-lg border border-border/70 bg-background/40 p-3">
              <div className="mb-1 text-xs uppercase tracking-widest text-muted-foreground">Contoh request</div>
              <pre className="overflow-x-auto text-[11px] leading-relaxed text-slate-300">{`curl -X POST http://localhost:8080/api/v1/payments \\
  -H "Authorization: Bearer ${showToken ? apiToken : "<API_TOKEN>"}" \\
  -H "Content-Type: application/json" \\
  -d '{"amount":15000,"reference":"ORDER-1"${
    form.default_provider_id ? "" : ',"provider_id":"<PROVIDER_ID>"'
  }}'`}</pre>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Webhook className="h-4 w-4 text-primary" /> Webhook
            </CardTitle>
            <CardDescription>Target notifikasi pembayaran lunas untuk semua provider.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-2">
              <Label>Webhook URL</Label>
              <Input
                value={form.webhook_url}
                onChange={(e) => setForm((s) => ({ ...s, webhook_url: e.target.value }))}
                placeholder="https://merchant.example.com/webhook"
              />
            </div>
            <div className="space-y-2">
              <Label>Webhook Secret (HMAC-SHA256)</Label>
              <Input
                value={form.webhook_secret}
                onChange={(e) => setForm((s) => ({ ...s, webhook_secret: e.target.value }))}
                className="font-mono text-xs"
                placeholder="secret untuk menandatangani payload"
              />
              <p className="text-xs text-muted-foreground">
                Header <code className="rounded bg-white/5 px-1">X-Gateway-Signature: sha256=&lt;hmac&gt;</code>{" "}
                dihitung dari raw body.
              </p>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Pembayaran</CardTitle>
          <CardDescription>Unique code & provider default untuk request tanpa provider_id.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="space-y-2">
              <Label>Kode unik min</Label>
              <Input
                type="number"
                value={form.unique_min}
                onChange={(e) => setForm((s) => ({ ...s, unique_min: Number(e.target.value) }))}
              />
            </div>
            <div className="space-y-2">
              <Label>Kode unik max</Label>
              <Input
                type="number"
                value={form.unique_max}
                onChange={(e) => setForm((s) => ({ ...s, unique_max: Number(e.target.value) }))}
              />
            </div>
            <div className="space-y-2">
              <Label>Provider default</Label>
              <Select
                value={form.default_provider_id}
                onChange={(e) => setForm((s) => ({ ...s, default_provider_id: e.target.value }))}
              >
                <option value="">Tanpa default (wajib provider_id)</option>
                {providers?.items.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.provider})
                  </option>
                ))}
              </Select>
            </div>
          </div>
          <Button onClick={save} disabled={busy}>
            <Save className="mr-2 h-4 w-4" /> Simpan Pengaturan
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
