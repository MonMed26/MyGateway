import type { Metadata } from "next";
import "./globals.css";
import { Toaster } from "@/components/ui/toaster";

export const metadata: Metadata = {
  title: "PayGateMe Gateway",
  description: "Semi payment gateway untuk Shopee & GoPay dengan QRIS dinamis, webhook, dan dashboard transaksi.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="id" className="dark">
      <body>
        {children}
        <Toaster />
      </body>
    </html>
  );
}
