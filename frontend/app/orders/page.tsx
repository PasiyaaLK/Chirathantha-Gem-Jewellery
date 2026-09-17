"use client";

// app/orders/page.tsx
//
// Order-tracking page (Section 5.1: "Order tracking page showing
// status: Pending Owner Review, Confirmed, In Progress, Shipped,
// Delivered, or Declined").

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { apiFetch, ApiError } from "@/lib/api";
import { formatCurrency, formatDate, titleCase } from "@/lib/format";
import { buttonSecondaryClass, pageShellClass, STATUS_BADGE } from "@/lib/styles";
import { useCurrentUser } from "@/lib/useAuth";
import type { Order, OrderItem } from "@/lib/types";

export default function OrdersPage() {
  const router = useRouter();
  const { user, loading: userLoading } = useCurrentUser();

  const [orders, setOrders] = useState<Order[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Same pattern as the admin dashboard: only redirect once we KNOW
  // there's no logged-in user, never while the token check is still in
  // flight.
  useEffect(() => {
    if (userLoading) return;
    if (!user) {
      router.replace("/login");
    }
  }, [user, userLoading, router]);

  useEffect(() => {
    if (userLoading || !user) return;
    apiFetch<Order[]>("/api/v1/orders", { auth: true })
      .then(setOrders)
      .catch((err) =>
        setError(err instanceof ApiError ? err.message : "Failed to load your orders."),
      );
  }, [userLoading, user]);

  if (userLoading || !user) {
    return (
      <div className={`${pageShellClass} flex items-center justify-center px-4 py-16`}>
        <p className="text-[#B8AD9E]">Checking your account…</p>
      </div>
    );
  }

  return (
    <div className={`${pageShellClass} px-4 py-12`}>
      <div className="mx-auto max-w-3xl">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 className="font-serif text-3xl">Your orders</h1>
            <p className="mt-2 text-[#B8AD9E]">
              Track the status of your custom and ready-made pieces.
            </p>
          </div>
          <Link href="/customize" className={buttonSecondaryClass}>
            Design something new
          </Link>
        </div>

        {error && <p className="mt-6 text-sm text-[#D96C55]">{error}</p>}

        {orders && orders.length === 0 && !error && (
          <p className="mt-10 text-[#B8AD9E]">
            No orders yet.{" "}
            <Link href="/customize" className="text-[#C9A46A] hover:underline">
              Start designing a piece
            </Link>
            .
          </p>
        )}

        {orders && orders.length > 0 && (
          <ul className="mt-8 space-y-3">
            {orders.map((order) => (
              <li key={order.id} className="rounded-md border border-[#332C25] bg-[#1F1B17] p-4">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <p className="font-medium">{describeOrder(order)}</p>
                    <p className="mt-1 text-sm text-[#B8AD9E]">
                      Order #{order.id.slice(0, 8)} · {formatDate(order.created_at)}
                    </p>
                  </div>
                  <div className="flex shrink-0 flex-col items-end gap-2">
                    <StatusBadge status={order.status} />
                    <span className="font-medium text-[#C9A46A]">
                      {formatCurrency(order.total_amount)}
                    </span>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function describeOrder(order: Order): string {
  const items = order.items ?? [];
  if (items.length === 0) return "Order";
  if (items.length === 1) return describeItem(items[0]);
  return `${describeItem(items[0])} + ${items.length - 1} more`;
}

function describeItem(item: OrderItem): string {
  if (item.customization) {
    const c = item.customization;
    const parts = [titleCase(c.category), titleCase(c.metal_type)];
    if (c.gemstone_type) parts.push(titleCase(c.gemstone_type));
    return parts.join(" — ");
  }
  if (item.product) return item.product.name;
  return "Item";
}

function StatusBadge({ status }: { status: Order["status"] }) {
  const badge = STATUS_BADGE[status];
  return (
    <span
      className={`whitespace-nowrap rounded-full border px-3 py-1 text-xs font-medium ${badge.className}`}
    >
      {badge.label}
    </span>
  );
}
