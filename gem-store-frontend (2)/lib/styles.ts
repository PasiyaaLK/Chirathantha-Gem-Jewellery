// lib/styles.ts
//
// Shared class strings and status-badge styling for the warm-dark,
// jewel-toned design system used across the app's pages, so
// login/signup/admin/orders style form controls identically instead of
// each page hand-rolling its own copy. (app/customize/page.tsx predates
// this file and still inlines its own copies of inputClass/labelClass —
// not touched here since refactoring it wasn't part of this step's ask,
// but it's a trivial follow-up to point it at this file instead.)

import type { OrderStatus } from "./types";

export const pageShellClass = "min-h-screen bg-[#161310] text-[#F3EDE2]";

export const inputClass =
  "mt-1 w-full rounded-md border border-[#332C25] bg-[#1F1B17] px-3 py-2 text-[#F3EDE2] " +
  "focus:border-[#C9A46A] focus:outline-none focus:ring-1 focus:ring-[#C9A46A] disabled:opacity-50";

export const labelClass = "block text-sm text-[#B8AD9E]";

export const buttonPrimaryClass =
  "rounded-md bg-[#8C3B4A] px-4 py-3 font-medium text-[#F3EDE2] transition-opacity " +
  "hover:opacity-90 disabled:opacity-40";

export const buttonSecondaryClass =
  "rounded-md border border-[#C9A46A] px-4 py-2 text-sm text-[#C9A46A] transition-colors " +
  "hover:bg-[#C9A46A] hover:text-[#161310] disabled:opacity-40";

export const buttonDangerOutlineClass =
  "rounded-md border border-[#D96C55] px-4 py-3 font-medium text-[#D96C55] transition-colors " +
  "hover:bg-[#D96C55] hover:text-[#161310] disabled:opacity-40";

/**
 * One badge style per order status, so color alone gives a rough sense
 * of where an order stands: muted for not-yet-decided or inactive
 * states, gold for "actively moving forward," soft sage for "arrived
 * safely," warm red for declined/cancelled. Deliberately not a default
 * traffic-light red/yellow/green — those colors already carry the
 * garnet/gold meanings established elsewhere in this palette.
 */
export const STATUS_BADGE: Record<OrderStatus, { label: string; className: string }> = {
  cart: {
    label: "Cart",
    className: "border-[#332C25] bg-[#1F1B17] text-[#B8AD9E]",
  },
  pending_review: {
    label: "Pending review",
    className: "border-[#6E6459] bg-[#1F1B17] text-[#D9C9A8]",
  },
  confirmed: {
    label: "Confirmed",
    className: "border-[#C9A46A]/50 bg-[#C9A46A]/10 text-[#C9A46A]",
  },
  declined: {
    label: "Declined",
    className: "border-[#D96C55]/50 bg-[#D96C55]/10 text-[#D96C55]",
  },
  in_progress: {
    label: "In progress",
    className: "border-[#C9A46A]/50 bg-[#C9A46A]/10 text-[#C9A46A]",
  },
  shipped: {
    label: "Shipped",
    className: "border-[#7FA88C]/50 bg-[#7FA88C]/10 text-[#7FA88C]",
  },
  delivered: {
    label: "Delivered",
    className: "border-[#7FA88C]/60 bg-[#7FA88C]/15 text-[#7FA88C]",
  },
  cancelled: {
    label: "Cancelled",
    className: "border-[#332C25] bg-[#1F1B17] text-[#6E6459]",
  },
};
