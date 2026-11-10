export type ProviderType = "shopee" | "gopay";

export interface Provider {
  id: string;
  name: string;
  provider: ProviderType;
  merchant_id: string;
  store_id: string;
  account_id: string;
  static_qris: string;
  poll_interval_ms: number;
  expiry_ms: number;
  clock_skew_ms: number;
  active: boolean;
  connected: boolean;
  created_at: string;
  updated_at: string;
}

export interface GlobalSettings {
  api_token: string;
  webhook_url: string;
  webhook_secret: string;
  unique_min: number;
  unique_max: number;
  default_provider_id: string;
}

export interface ProviderStatus {
  id: string;
  name: string;
  provider: ProviderType;
  connected: boolean;
  authenticated: boolean;
  merchant_id: string;
  store_id: string;
  account_id: string;
  merchant_name?: string;
  store_name?: string;
  outlet_id?: string;
  has_qris: boolean;
  polling: boolean;
  error?: string;
}

export interface Stats {
  total_payments: number;
  paid_payments: number;
  pending: number;
  expired: number;
  cancelled: number;
  paid_volume: number;
  providers: number;
  connected: number;
}

export interface MerchantTransaction {
  ID?: string;
  OrderID?: string;
  Status?: string;
  PaymentType?: string;
  Amount?: number;
  RealAmount?: number;
  Currency?: string;
  Time?: string;
}

export interface Payment {
  id: string;
  config_id: string;
  reference: string;
  status: "pending" | "paid" | "expired" | "cancelled" | string;
  base_amount: number;
  unique_offset: number;
  unique_amount: number;
  qr_string?: string;
  created_at: string;
  expires_at: string;
  paid_at?: string;
  transaction?: MerchantTransaction;
  metadata?: Record<string, unknown>;
}

export interface Delivery {
  id: string;
  config_id: string;
  payment_id: string;
  url: string;
  status: "pending" | "success" | "failed" | string;
  signature: string;
  attempts: number;
  last_status_code: number;
  last_response: string;
  next_attempt_at: string;
  created_at: string;
  updated_at: string;
}

export interface TransactionsResponse {
  items: Payment[];
  total: number;
  page: number;
  page_size: number;
}

export interface TransactionDetailResponse {
  payment: Payment;
  webhooks: Delivery[];
}

export interface StoreItem {
  id: string;
  name: string;
  status?: number;
}

export interface GoPayMerchant {
  id: string;
  merchantName: string;
  outlets?: Array<{ popId: string; name?: string; qrString?: string }>;
}
