"use client";

// app/admin/dashboard/page.tsx
//
// Protected admin queue for custom orders awaiting review (Section 5.2:
// "Admin dashboard summarizing pending custom orders" +
// "Custom order review screen ... with Confirm / Decline actions").

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { apiFetch, ApiError, logout } from "@/lib/api";
import { formatCurrency, formatDate, formatShippingAddress, titleCase } from "@/lib/format";
import {
  buttonDangerOutlineClass,
  buttonPrimaryClass,
  inputClass,
  labelClass,
  pageShellClass,
} from "@/lib/styles";
import { useCurrentUser } from "@/lib/useAuth";
import type { DecisionRequest, PendingCustomOrder } from "@/lib/types";

export default function AdminDashboardPage() {
  const router = useRouter();
  const { user, loading: userLoading } = useCurrentUser();

  const [orders, setOrders] = useState<PendingCustomOrder[] | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [selected, setSelected] = useState<PendingCustomOrder | null>(null);

  const isAdmin = !userLoading && user?.role === "admin";

  // Redirect only once we KNOW the user isn't an admin — never before
  // userLoading settles. Redirecting while the token check is still in
  // flight would send a legitimately logged-in admin to /login on every
  // page load, on nothing but the fact that the check hadn't finished
  // yet.
  useEffect(() => {
    if (userLoading) return;
    if (!user || user.role !== "admin") {
      router.replace("/login");
    }
  }, [user, userLoading, router]);

  useEffect(() => {
    if (!isAdmin) return;
    void refreshOrders();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAdmin]);

  async function refreshOrders() {
    setListError(null);
    try {
      const data = await apiFetch<PendingCustomOrder[]>("/api/v1/admin/orders/pending");
      setOrders(data);
    } catch (err) {
      setListError(err instanceof ApiError ? err.message : "Failed to load pending orders.");
    }
  }

  async function handleDecision(orderId: string, decision: "approve" | "decline", notes: string) {
    const body: DecisionRequest = notes.trim() ? { notes: notes.trim() } : {};
    await apiFetch(`/api/v1/admin/orders/${orderId}/${decision}`, {
      method: "POST",
      body,
    });
    setSelected(null);
    await refreshOrders();
  }

  // Covers both "still checking the token" and the brief moment before
  // the redirect effect above fires for a non-admin — the dashboard's
  // real content never renders in either case.
  if (userLoading || !isAdmin) {
    return (
      <div className={`${pageShellClass} flex items-center justify-center px-4 py-16`}>
        <p className="text-[#B8AD9E]">Checking your access…</p>
      </div>
    );
  }

  return (
    <div className={`${pageShellClass} px-4 py-12`}>
      <div className="mx-auto max-w-5xl">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="font-serif text-3xl">Pending custom orders</h1>
            <p className="mt-2 text-[#B8AD9E]">
              Review each design before it moves to production. Confirming notifies
              the customer automatically; declining requires a reason.
            </p>
          </div>
          <button
            onClick={() => {
              void logout().then(() => router.push("/login"));
            }}
            className="shrink-0 text-sm text-[#B8AD9E] underline-offset-4 hover:text-[#F3EDE2] hover:underline"
          >
            Log out
          </button>
        </div>

        {listError && <p className="mt-6 text-sm text-[#D96C55]">{listError}</p>}

        {orders && orders.length === 0 && !listError && (
          <p className="mt-10 text-[#B8AD9E]">Nothing waiting on review right now.</p>
        )}

        {orders && orders.length > 0 && (
          <div className="mt-8 divide-y divide-[#332C25] border-y border-[#332C25]">
            {orders.map((o) => (
              <button
                key={o.order_id}
                onClick={() => setSelected(o)}
                className="flex w-full items-center justify-between gap-4 px-2 py-4 text-left transition-colors hover:bg-[#1F1B17]"
              >
                <div>
                  <p className="font-medium">
                    {titleCase(o.customization.category)} — {titleCase(o.customization.metal_type)}
                    {o.customization.gemstone_type ? `, ${titleCase(o.customization.gemstone_type)}` : ""}
                  </p>
                  <p className="text-sm text-[#B8AD9E]">
                    {o.customer_name} · {formatDate(o.created_at)}
                  </p>
                </div>
                <span className="whitespace-nowrap text-lg font-medium text-[#C9A46A]">
                  {formatCurrency(o.total_amount)}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>

      {selected && (
        <OrderInspectionDrawer
          order={selected}
          onClose={() => setSelected(null)}
          onDecide={handleDecision}
        />
      )}
    </div>
  );
}

function OrderInspectionDrawer({
  order,
  onClose,
  onDecide,
}: {
  order: PendingCustomOrder;
  onClose: () => void;
  onDecide: (orderId: string, decision: "approve" | "decline", notes: string) => Promise<void>;
}) {
  const [notes, setNotes] = useState("");
  const [pending, setPending] = useState<"approve" | "decline" | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const c = order.customization;

  async function act(decision: "approve" | "decline") {
    setActionError(null);

    // Mirrors ApprovalService.DeclineCustomOrder's server-side rule —
    // checked here for immediate feedback; the server enforces it
    // regardless of what the client does.
    if (decision === "decline" && !notes.trim()) {
      setActionError("A reason is required when declining an order.");
      return;
    }

    setPending(decision);
    try {
      await onDecide(order.order_id, decision, notes);
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : `Failed to ${decision} the order.`);
    } finally {
      setPending(null);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60" onClick={onClose}>
      <div
        className="flex h-full w-full max-w-md flex-col overflow-y-auto border-l border-[#332C25] bg-[#161310] p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h2 className="font-serif text-2xl">Order details</h2>
          <button
            onClick={onClose}
            aria-label="Close"
            className="text-[#B8AD9E] transition-colors hover:text-[#F3EDE2]"
          >
            ✕
          </button>
        </div>

        <dl className="mt-6 space-y-3 text-sm">
          <Row label="Customer" value={`${order.customer_name} (${order.customer_email})`} />
          {order.customer_phone && <Row label="Phone" value={order.customer_phone} />}
          <Row label="Category" value={titleCase(c.category)} />
          <Row label="For" value={titleCase(c.gender)} />
          <Row label="Metal" value={titleCase(c.metal_type)} />
          <Row label="Gemstone" value={c.gemstone_type ? titleCase(c.gemstone_type) : "None"} />
          {c.size && <Row label="Size" value={c.size} />}
          {c.engraving_text && <Row label="Engraving" value={`\u201c${c.engraving_text}\u201d`} />}
          <Row label="Quantity" value={String(order.quantity)} />
          <Row label="Shipping address" value={formatShippingAddress(order.shipping_address)} />
        </dl>

        <div className="mt-6 flex items-center justify-between border-t border-[#332C25] pt-4">
          <span className="text-[#B8AD9E]">Total</span>
          <span className="text-xl font-medium text-[#C9A46A]">{formatCurrency(order.total_amount)}</span>
        </div>

        <div className="mt-6">
          <label className={labelClass} htmlFor="admin-notes">
            Notes <span className="text-[#6E6459]">(required when declining)</span>
          </label>
          <textarea
            id="admin-notes"
            rows={3}
            className={inputClass}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="e.g. estimated ship date, or reason for declining"
          />
        </div>

        {actionError && <p className="mt-3 text-sm text-[#D96C55]">{actionError}</p>}

        <div className="mt-6 flex gap-3">
          <button
            onClick={() => act("approve")}
            disabled={pending !== null}
            className={`${buttonPrimaryClass} flex-1`}
          >
            {pending === "approve" ? "Approving…" : "Approve"}
          </button>
          <button
            onClick={() => act("decline")}
            disabled={pending !== null}
            className={`${buttonDangerOutlineClass} flex-1`}
          >
            {pending === "decline" ? "Declining…" : "Decline"}
          </button>
        </div>
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="shrink-0 text-[#B8AD9E]">{label}</dt>
      <dd className="text-right">{value}</dd>
    </div>
  );
}
