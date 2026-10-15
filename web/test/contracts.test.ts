import { describe, expect, it } from "vitest";
import { ApiError, GalerikuClient } from "../src/client.js";
import {
  AUTH_ERROR_CODES,
  CAPABILITY_CODES,
  EVENT_STATUSES,
  FULFILLMENT_STATUSES,
  HOST_MEMBERSHIP_ROLES,
  HOST_APPLICATION_STATUSES,
  HOST_DOCUMENT_TYPES,
  LEDGER_DIRECTIONS,
  MAX_FILES_PER_BATCH,
  ORDER_STATUSES,
  PAYMENT_STATUSES,
  SESSION_STATUSES,
  SETTLEMENT_STATUSES,
  UPLOAD_BATCH_STATUSES,
  isAuthErrorCode,
  isCapabilityCode,
} from "../src/types/api.js";

describe("Domain enum taxonomy across all Domains 01-06", () => {
  it("matches the backend capability set exactly", () => {
    expect([...CAPABILITY_CODES].sort()).toEqual(
      ["BUYER", "CREATOR", "HOST_ADMIN", "HOST_LEADER", "SUPER_ADMIN"].sort(),
    );
  });

  it("narrows unknown values", () => {
    expect(isCapabilityCode("HOST_LEADER")).toBe(true);
    expect(isCapabilityCode("ROOT")).toBe(false);
    expect(isAuthErrorCode("ORDER_NOT_PAYABLE")).toBe(true);
    expect(isAuthErrorCode("MADE_UP_CODE")).toBe(false);
  });

  it("keeps core enums aligned with the Go constants", () => {
    expect([...HOST_MEMBERSHIP_ROLES]).toEqual(["LEADER", "ADMIN"]);
    expect([...HOST_APPLICATION_STATUSES]).toEqual(["SUBMITTED", "APPROVED", "REJECTED"]);
    expect([...HOST_DOCUMENT_TYPES]).toEqual(["KTP_DIREKSI", "NPWP_BADAN", "NIB_OSS", "AKTA_PENDIRIAN"]);
    expect([...EVENT_STATUSES]).toEqual(["DRAFT", "PUBLISHED", "ARCHIVED"]);
    expect([...SESSION_STATUSES]).toEqual(["SCHEDULED", "ACTIVE", "COMPLETED"]);
    expect([...UPLOAD_BATCH_STATUSES]).toEqual(["RESERVED", "COMPLETED", "EXPIRED"]);
    expect([...ORDER_STATUSES]).toEqual(["PENDING_PAYMENT", "PAID", "CANCELLED"]);
    expect([...PAYMENT_STATUSES]).toEqual(["PENDING", "SETTLED", "FAILED", "EXPIRED"]);
    expect([...FULFILLMENT_STATUSES]).toEqual(["WAITING_MASTER", "FULFILLED", "SLA_BREACH"]);
    expect([...LEDGER_DIRECTIONS]).toEqual(["DEBIT", "CREDIT"]);
    expect([...SETTLEMENT_STATUSES]).toEqual(["HOLDING", "RELEASED"]);
    expect(MAX_FILES_PER_BATCH).toBe(2000);
  });

  it("carries the full cross-domain error catalog", () => {
    for (const code of [
      "SLUG_ALREADY_EXISTS",
      "QUOTA_EXCEEDED",
      "ORDER_NOT_PAYABLE",
      "DOWNLOAD_NOT_PERMITTED",
      "BATCH_NOT_FINALIZABLE",
      "IDEMPOTENCY_KEY_CONFLICT",
      "APPLICATION_ALREADY_REVIEWED",
      "MEMBERSHIP_ALREADY_EXISTS",
    ]) {
      expect(AUTH_ERROR_CODES).toContain(code);
    }
  });
});

describe("GalerikuClient", () => {
  const okEnvelope = (data: unknown) => ({
    ok: true,
    json: async () => ({ success: true, data, meta: { request_id: "req_test" } }),
    status: 200,
  });

  it("sends the bearer token and parses success envelopes", async () => {
    let captured: Record<string, string> = {};
    const fetchImpl = (async (_url: any, init: any) => {
      captured = init.headers;
      return okEnvelope({ cart_id: "c1", total_items: 0, total_amount_idr: 0, items: [] });
    }) as unknown as typeof fetch;

    const client = new GalerikuClient({ baseUrl: "http://test", fetchImpl });
    client.setAccessToken("tok123");
    const cart = await client.getCart();

    expect(captured["Authorization"]).toBe("Bearer tok123");
    expect(cart.cart_id).toBe("c1");
  });

  it("sends the Idempotency-Key header on order creation", async () => {
    let captured: Record<string, string> = {};
    const fetchImpl = (async (_url: any, init: any) => {
      captured = init.headers;
      return okEnvelope({ order_id: "o1", order_number: "GLK-x", total_amount_idr: 100, status: "PENDING_PAYMENT" });
    }) as unknown as typeof fetch;

    const client = new GalerikuClient({ baseUrl: "http://test", fetchImpl });
    await client.createOrder(["p1"], "key-42");

    expect(captured["Idempotency-Key"]).toBe("key-42");
  });

  it("throws ApiError with the backend code for error envelopes", async () => {
    const fetchImpl = (async () => ({
      ok: false,
      status: 422,
      json: async () => ({
        success: false,
        error: { code: "QUOTA_EXCEEDED", message: "insufficient quota", request_id: "req_1" },
        meta: { timestamp: "t", request_id: "req_1" },
      }),
    })) as unknown as typeof fetch;

    const client = new GalerikuClient({ baseUrl: "http://test", fetchImpl });
    const err = await client.addToCart("p1").catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.status).toBe(422);
    expect(apiErr.code).toBe("QUOTA_EXCEEDED");
  });

  it("builds correct paths for all resource families", async () => {
    const paths: string[] = [];
    const fetchImpl = (async (url: any) => {
      paths.push(String(url));
      return okEnvelope({});
    }) as unknown as typeof fetch;

    const client = new GalerikuClient({ baseUrl: "http://test/", fetchImpl });
    await client.searchBIB("10452");
    await client.getCart();
    await client.getCart();
    await client.initiatePayment("o1", "QRIS");
    await client.grantDownload("oi1", "NON_HIRES_DOWNLOAD");
    await client.uploadMaster("oi1", "k", "s");
    await client.finalizeBatch("b1");
    await client.acceptInvitation("inv");

    expect(paths.some((p) => p.endsWith("/api/v1/search/bib"))).toBe(true);
    expect(paths.some((p) => p.endsWith("/api/v1/downloads/grant/oi1"))).toBe(true);
    expect(paths.some((p) => p.endsWith("/api/v1/fulfillment/upload-master/oi1"))).toBe(true);
    expect(paths.some((p) => p.endsWith("/api/v1/storage/upload-batches/b1/finalize"))).toBe(true);
    expect(paths.some((p) => p.endsWith("/api/v1/hosts/invitations/accept"))).toBe(true);
  });
});
