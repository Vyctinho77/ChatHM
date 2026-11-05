import { AlertCircle, Check, CheckCheck, Clock } from "lucide-react";
import { cn } from "@/lib/cn";
import type { DeliveryStatus } from "./types";

const labels: Record<DeliveryStatus, string> = {
  pending: "Enviando",
  sent: "Enviada",
  delivered: "Entregue",
  read: "Lida",
  failed: "Falha no envio",
};

export function Ticks({ status, className }: { status?: DeliveryStatus; className?: string }) {
  if (!status) return null;
  const cls = cn("size-4 shrink-0", className);
  const label = labels[status];
  switch (status) {
    case "pending":
      return <Clock className={cn(cls, "size-3.5 opacity-70")} aria-label={label} />;
    case "sent":
      return <Check className={cn(cls, "opacity-70")} aria-label={label} />;
    case "delivered":
      return <CheckCheck className={cn(cls, "opacity-70")} aria-label={label} />;
    case "read":
      return <CheckCheck className={cn(cls, "text-read")} aria-label={label} />;
    case "failed":
      return <AlertCircle className={cn(cls, "text-danger")} aria-label={label} />;
  }
}
