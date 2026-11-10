"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import useSWR from "swr";
import { ArrowLeft, Loader2, RefreshCw, Save, ShieldCheck, Zap } from "lucide-react";
import { toast } from "sonner";
import { AppShell } from "@/components/app-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { api, fetcher } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { GoPayMerchant, Provider, ProviderStatus, StoreItem } from "@/lib/types";
import { useQueryParam } from "@/lib/use-query";

export default function ProviderDetailPage() {
  return (
    <AppShell>
      <ProviderDetail />
    </AppShell>
  );
}

type ProviderLike = { id: string; name: string };
type MerchantChoice = ProviderLike & { merchantName?: string };

function ProviderDetail() {
  const id = useQueryParam("id");
  const { data: provider, mutate: mutateProvider, error: providerError } = useSWR<Provider>(
    id ? `/api/admin/providers/${id}` : null,
    fetcher,
  );
  const { data: status, mutate: mutateStatus } = useSWR<ProviderStatus>(
    id ? `/api/admin/providers/${id}/status` : null,
    fetcher,
    { refreshInterval: 6000 },
  );

  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [otp, setOtp] = useState("");
  const [otpToken, setOtpToken] = useState("");
  const [otpSent, setOtpSent] = useState(false);
  const [pendingMerchants, setPendingMerchants] = useState<MerchantChoice[] | null>(null);
  const [merchantChoice, setMerchantChoice] = useState("");
  const [outletChoice, setOutletChoice] = useState("");
  const [busy, setBusy] = useState(false);
  const [otpError, setOtpError] = useState<string | null>(null);
  const [qris, setQris] = useState("");
  const [settings, setSettings] = useState({
    poll_interval_ms: 3000,
    expiry_ms: 300000,
    clock_skew_ms: 60000,
    active: true,
  });

  useEffect(() => {
    if (provider) {
      setSettings({
        poll_interval_ms: provider.poll_interval_ms,
        expiry_ms: provider.expiry_ms,
        clock_skew_ms: provider.clock_skew_ms,
        active: provider.active,
      });
      setQris(provider.static_qris ?? "");
    }
  }, [provider]);

  const authenticated = Boolean(status?.authenticated);
  const { data: stores } = useSWR<{ items: StoreItem[] }>(
    id && authenticated && provider?.provider === "shopee" ? `/api/admin/providers/${id}/stores` : null,
    fetcher,
  );
  const { data: merchants } = useSWR<{ items: GoPayMerchant[] }>(
    id && authenticated && provider?.provider === "gopay" ? `/api/admin/providers/${id}/merchants` : null,
    fetcher,
  );

  async function refresh() {
    await Promise.all([mutateProvider(), mutateStatus()]);
  }

  function fail(err: unknown) {
    toast.error(err instanceof Error ? err.message : "Terjadi kesalahan");
  }

  function failOtp(err: unknown) {
    const message = err instanceof Error ? err.message : "Terjadi kesalahan";
    setOtpError(message);
    toast.error(message);
  }

  async function requestOtp() {
    if (!id) return;
    setBusy(true);
    setOtpError(null);
    try {
      const res = await api.post<{ provider: string; otp_token?: string; channel?: number }>(
        `/api/admin/providers/${id}/otp/request`,
        { phone, password },
      );
      setOtpSent(true);
      if (res.otp_token) setOtpToken(res.otp_token);
      toast.success(
        res.provider === "shopee"
          ? `OTP dikirim (channel ${res.channel ?? "-"})`
          : "OTP dikirim. Masukkan kode yang diterima.",
      );
    } catch (err) {
      failOtp(err);
    } finally {
      setBusy(false);
    }
  }

  async function verifyOtp() {
    if (!id) return;
    setBusy(true);
    setOtpError(null);
    try {
      const res = await api.post<{ status: string; merchants?: MerchantChoice[] }>(
        `/api/admin/providers/${id}/otp/verify`,
        { otp, otp_token: otpToken },
      );
      if (res.status === "merchant_selection") {
        setPendingMerchants(res.merchants ?? []);
        toast.info("Pilih merchant untuk melanjutkan.");
      } else {
        toast.success("Login berhasil");
        setOtpSent(false);
        setOtp("");
        await refresh();
      }
    } catch (err) {
      failOtp(err);
    } finally {
      setBusy(false);
    }
  }

  async function completeLogin() {
    if (!id || !merchantChoice) return;
    setBusy(true);
    setOtpError(null);
    try {
      await api.post(`/api/admin/providers/${id}/complete-login`, { merchant_id: merchantChoice });
      toast.success("Merchant dipilih, login selesai");
      setPendingMerchants(null);
      setOtpSent(false);
      await refresh();
    } catch (err) {
      failOtp(err);
    } finally {
      setBusy(false);
    }
  }

  async function selectStore(storeId: string) {
    if (!id) return;
    setBusy(true);
    try {
      await api.post(`/api/admin/providers/${id}/select-store`, { store_id: storeId });
      toast.success("Store dipilih");
      await refresh();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function selectMerchant(merchantId: string) {
    if (!id) return;
    setBusy(true);
    try {
      await api.post(`/api/admin/providers/${id}/select-merchant`, { merchant_id: merchantId });
      toast.success("Merchant dipilih");
      setMerchantChoice(merchantId);
      await refresh();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function selectOutlet(popId: string) {
    if (!id) return;
    setBusy(true);
    try {
      await api.post(`/api/admin/providers/${id}/select-outlet`, { pop_id: popId });
      toast.success("Outlet dipilih");
      await refresh();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function refreshSession() {
    if (!id) return;
    setBusy(true);
    try {
      await api.post(`/api/admin/providers/${id}/refresh`);
      toast.success("Session di-refresh");
      await refresh();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function saveSettings() {
    if (!id) return;
    setBusy(true);
    try {
      await api.patch(`/api/admin/providers/${id}`, settings);
      toast.success("Pengaturan disimpan");
      await refresh();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function saveQris() {
    if (!id) return;
    setBusy(true);
    try {
      await api.post(`/api/admin/providers/${id}/qris`, { payload: qris.trim() });
      toast.success("QRIS disimpan");
      await refresh();
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  if (providerError) {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
        Provider tidak ditemukan.
      </div>
    );
  }

  if (!provider || !status) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const selectedMerchant = merchants?.items.find((m) => m.id === merchantChoice);

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <Button asChild variant="ghost" size="icon" className="mt-1">
            <Link href="/providers">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">{provider.name}</h1>
              <Badge variant="secondary" className="capitalize">
                {provider.provider}
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground">
              ID {provider.id} · dibuat {formatDate(provider.created_at)}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={status.connected ? "default" : "outline"}>
            {status.connected ? "Session tersimpan" : "Belum login"}
          </Badge>
          <Badge variant={status.authenticated ? "default" : "outline"}>
            {status.authenticated ? "Authenticated" : "Unauthenticated"}
          </Badge>
          <Badge variant={status.polling ? "default" : "outline"}>
            {status.polling ? "Polling aktif" : "Polling berhenti"}
          </Badge>
        </div>
      </div>

      {status.error && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-2 text-sm text-amber-200">
          {status.error}
        </div>
      )}

      <div className="grid gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="h-4 w-4 text-primary" /> Koneksi Merchant
            </CardTitle>
            <CardDescription>
              {authenticated
                ? "Session aktif. Kelola store/merchant dan refresh token di sini."
                : "Masukkan nomor HP akun merchant untuk menerima OTP."}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {otpError && (
              <div className="whitespace-pre-wrap break-words rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                {otpError}
              </div>
            )}
            {authenticated ? (
              <>
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <Info label="Merchant" value={status.merchant_name || status.merchant_id || "—"} />
                  <Info label="Store / Outlet" value={status.store_name || status.store_id || status.outlet_id || "—"} />
                  <Info label="QRIS" value={status.has_qris ? "Terpasang" : "Belum diatur"} />
                  <Info label="Polling" value={status.polling ? "Aktif" : "Berhenti"} />
                </div>

                {provider.provider === "shopee" && (
                  <>
                    <Separator />
                    <div className="space-y-2">
                      <Label>Ganti Store</Label>
                      <div className="flex gap-2">
                        <Select value={merchantChoice} onChange={(e) => setMerchantChoice(e.target.value)}>
                          <option value="">Pilih store…</option>
                          {stores?.items.map((s) => (
                            <option key={s.id} value={s.id}>
                              {s.name} ({s.id})
                            </option>
                          ))}
                        </Select>
                        <Button
                          variant="outline"
                          disabled={!merchantChoice || busy}
                          onClick={() => selectStore(merchantChoice)}
                        >
                          Simpan
                        </Button>
                      </div>
                    </div>
                  </>
                )}

                {provider.provider === "gopay" && (
                  <>
                    <Separator />
                    <div className="space-y-2">
                      <Label>Pilih Merchant</Label>
                      <div className="flex gap-2">
                        <Select value={merchantChoice} onChange={(e) => setMerchantChoice(e.target.value)}>
                          <option value="">Pilih merchant…</option>
                          {merchants?.items.map((m) => (
                            <option key={m.id} value={m.id}>
                              {m.merchantName} ({m.id})
                            </option>
                          ))}
                        </Select>
                        <Button
                          variant="outline"
                          disabled={!merchantChoice || busy}
                          onClick={() => selectMerchant(merchantChoice)}
                        >
                          Pilih
                        </Button>
                      </div>
                    </div>
                    {selectedMerchant?.outlets && selectedMerchant.outlets.length > 0 && (
                      <div className="space-y-2">
                        <Label>Pilih Outlet</Label>
                        <div className="flex gap-2">
                          <Select value={outletChoice} onChange={(e) => setOutletChoice(e.target.value)}>
                            <option value="">Pilih outlet…</option>
                            {selectedMerchant.outlets.map((o) => (
                              <option key={o.popId} value={o.popId}>
                                {o.name || o.popId}
                              </option>
                            ))}
                          </Select>
                          <Button
                            variant="outline"
                            disabled={!outletChoice || busy}
                            onClick={() => selectOutlet(outletChoice)}
                          >
                            Simpan
                          </Button>
                        </div>
                      </div>
                    )}
                  </>
                )}

                <Button variant="outline" onClick={refreshSession} disabled={busy} className="w-full">
                  <RefreshCw className={`mr-2 h-4 w-4 ${busy ? "animate-spin" : ""}`} /> Refresh Session
                </Button>
              </>
            ) : (
              <>
                <div className="space-y-2">
                  <Label>Nomor HP</Label>
                  <Input
                    value={phone}
                    onChange={(e) => setPhone(e.target.value)}
                    placeholder="081234567890"
                    inputMode="tel"
                  />
                </div>
                {provider.provider === "shopee" && (
                  <div className="space-y-2">
                    <Label>Password (opsional)</Label>
                    <Input
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      placeholder="Kosongkan bila passwordless"
                    />
                  </div>
                )}
                <Button onClick={requestOtp} disabled={!phone || busy} className="w-full">
                  {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Zap className="mr-2 h-4 w-4" />}
                  Kirim OTP
                </Button>

                {otpSent && (
                  <div className="space-y-3 rounded-lg border border-border/70 p-3">
                    <div className="space-y-2">
                      <Label>Kode OTP</Label>
                      <Input
                        value={otp}
                        onChange={(e) => setOtp(e.target.value)}
                        placeholder="123456"
                        inputMode="numeric"
                      />
                    </div>
                    <Button onClick={verifyOtp} disabled={!otp || busy} className="w-full">
                      {busy ? "Memverifikasi…" : "Verifikasi OTP"}
                    </Button>
                  </div>
                )}

                {pendingMerchants && pendingMerchants.length > 0 && (
                  <div className="space-y-2 rounded-lg border border-border/70 p-3">
                    <Label>Pilih Merchant</Label>
                    <Select value={merchantChoice} onChange={(e) => setMerchantChoice(e.target.value)}>
                      <option value="">Pilih merchant…</option>
                      {pendingMerchants.map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name} ({m.id})
                        </option>
                      ))}
                    </Select>
                    <Button onClick={completeLogin} disabled={!merchantChoice || busy} className="w-full">
                      Selesaikan Login
                    </Button>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Pengaturan Pembayaran</CardTitle>
            <CardDescription>
              Interval polling, expiry, dan status provider. Kode unik & webhook diatur global di menu Pengaturan.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label>Poll interval (ms)</Label>
                <Input
                  type="number"
                  value={settings.poll_interval_ms}
                  onChange={(e) => setSettings((s) => ({ ...s, poll_interval_ms: Number(e.target.value) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Expiry (ms)</Label>
                <Input
                  type="number"
                  value={settings.expiry_ms}
                  onChange={(e) => setSettings((s) => ({ ...s, expiry_ms: Number(e.target.value) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Clock skew (ms)</Label>
                <Input
                  type="number"
                  value={settings.clock_skew_ms}
                  onChange={(e) => setSettings((s) => ({ ...s, clock_skew_ms: Number(e.target.value) }))}
                />
              </div>
              <div className="flex items-end justify-between rounded-md border border-border/70 px-3 py-2">
                <span className="text-sm">Aktif</span>
                <Switch
                  checked={settings.active}
                  onCheckedChange={(v) => setSettings((s) => ({ ...s, active: v }))}
                />
              </div>
            </div>
            <Button onClick={saveSettings} disabled={busy} className="w-full">
              <Save className="mr-2 h-4 w-4" /> Simpan Pengaturan
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Static QRIS</CardTitle>
            <CardDescription>Payload EMVCo statis; akan dijadikan QRIS dinamis per transaksi.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Textarea
              value={qris}
              onChange={(e) => setQris(e.target.value)}
              placeholder="00020101021126…"
              className="min-h-[150px] font-mono text-xs"
            />
            <Button onClick={saveQris} disabled={busy || !qris.trim()} className="w-full">
              <Save className="mr-2 h-4 w-4" /> Simpan QRIS
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border/70 bg-background/40 px-3 py-2">
      <div className="text-[11px] uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className="truncate text-sm">{value}</div>
    </div>
  );
}
