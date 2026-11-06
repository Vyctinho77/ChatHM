import { cn } from "@/lib/cn";
import logo from "@/assets/brand/logo.svg";
import logoDark from "@/assets/brand/logo-dark.svg";

export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex shrink-0 items-center align-middle", className)}>
      <img src={logo} alt="ChatHM" width={373} height={86} className="block h-8 w-auto dark:hidden" />
      <img src={logoDark} alt="ChatHM" width={373} height={86} className="hidden h-8 w-auto dark:block" />
    </span>
  );
}
