import { AnimatePresence, motion } from "motion/react";
import { AlertCircle, Check, CheckCheck, Clock } from "lucide-react";
import { cn } from "@/lib/cn";
import { statusLabel } from "./receipts";
import type { DeliveryStatus } from "./types";

function Icon({ status, className }: { status: DeliveryStatus; className?: string }) {
  const cls = cn("size-4 shrink-0", className);
  switch (status) {
    case "pending":
      return <Clock className={cn(cls, "size-3.5 opacity-70")} />;
    case "sent":
      return <Check className={cn(cls, "opacity-70")} />;
    case "delivered":
      return <CheckCheck className={cn(cls, "opacity-70")} />;
    case "read":
      return <CheckCheck className={cn(cls, "text-read")} />;
    case "failed":
      return <AlertCircle className={cn(cls, "text-danger")} />;
  }
}

/** Ícone de status com transição suave a cada mudança (enviando → enviada → entregue → lida). */
export function Ticks({ status, className }: { status?: DeliveryStatus; className?: string }) {
  if (!status) return null;
  return (
    <span className="relative inline-flex size-4 shrink-0 items-center justify-center" aria-label={statusLabel[status]} role="img">
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={status}
          className="inline-flex"
          initial={{ opacity: 0, scale: 0.5 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, scale: 0.5 }}
          transition={{ type: "spring", stiffness: 520, damping: 26 }}
        >
          <Icon status={status} className={className} />
        </motion.span>
      </AnimatePresence>
    </span>
  );
}
