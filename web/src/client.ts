/**
 * Typed API client for the Galeriku backend. Every call returns the parsed
 * envelope and preserves the standard error shape. The client is transport-
 * agnostic: point baseUrl at localhost:8080 in dev, the gateway in prod.
 */
import type {
  AcceptInvitationResult,
  BatchFinalizeResult,
  BIBSearchResult,
  CartResult,
  DownloadGrantResult,
  ErrorEnvelope,
  HostApplicationResult,
  HostInvitationResult,
  MultipartInitiateResult,
  MultipartCompleteResult,
  PaymentInitiationResult,
  SuccessEnvelope,
  UploadBatchResult,
  UploadMasterResult,
} from "./types/api.js";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export interface ClientOptions {
  baseUrl: string;
  fetchImpl?: typeof fetch;
}

export class GalerikuClient {
  private readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;
  private accessToken: string | null = null;

  constructor(options: ClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.fetchImpl = options.fetchImpl ?? fetch;
  }

  setAccessToken(token: string | null): void {
    this.accessToken = token;
  }

  private async request<T>(method: string, path: string, body?: unknown, idempotencyKey?: string): Promise<T> {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (this.accessToken) {
      headers["Authorization"] = `Bearer ${this.accessToken}`;
    }
    if (idempotencyKey) {
      headers["Idempotency-Key"] = idempotencyKey;
    }

    const response = await this.fetchImpl(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });

    const payload = (await response.json()) as SuccessEnvelope<T> | ErrorEnvelope;
    if (!payload.success) {
      const err = payload as ErrorEnvelope;
      throw new ApiError(response.status, err.error.code, err.error.message, err.error.request_id ?? err.meta?.request_id);
    }
    return (payload as SuccessEnvelope<T>).data;
  }

  // Discovery (public)

  searchBIB(bibNumber: string, eventId?: string): Promise<BIBSearchResult> {
    return this.request("POST", "/api/v1/search/bib", { bib_number: bibNumber, event_id: eventId });
  }

  // Cart and orders

  getCart(): Promise<CartResult> {
    return this.request("GET", "/api/v1/cart");
  }

  addToCart(photoId: string): Promise<{ cart_item_id: string; photo_id: string; price_idr: number }> {
    return this.request("POST", "/api/v1/cart/items", { photo_id: photoId });
  }

  removeFromCart(cartItemId: string): Promise<{ deleted: boolean }> {
    return this.request("DELETE", `/api/v1/cart/items/${cartItemId}`);
  }

  createOrder(photoIds: string[], idempotencyKey: string, bibGrants?: string[]): Promise<{ order_id: string; order_number: string; total_amount_idr: number; status: string }> {
    return this.request("POST", "/api/v1/orders", { photo_ids: photoIds, bib_grants: bibGrants ?? [] }, idempotencyKey);
  }

  // Payments and downloads

  initiatePayment(orderId: string, method: string): Promise<PaymentInitiationResult> {
    return this.request("POST", "/api/v1/payments/initiate", { order_id: orderId, payment_method: method });
  }

  grantDownload(orderItemId: string, assetType: string): Promise<DownloadGrantResult> {
    return this.request("POST", `/api/v1/downloads/grant/${orderItemId}`, { requested_asset: assetType });
  }

  uploadMaster(orderItemId: string, objectKey: string, sha256: string): Promise<UploadMasterResult> {
    return this.request("POST", `/api/v1/fulfillment/upload-master/${orderItemId}`, {
      storage_object_key: objectKey,
      sha256_checksum: sha256,
    });
  }

  // Host application

  applyAsHost(input: { legal_name: string; display_name: string; business_field: string; address_text?: string }): Promise<HostApplicationResult> {
    return this.request("POST", "/api/v1/hosts/apply", input);
  }

  inviteTeamMember(invitedEmail: string, membershipRole: string): Promise<HostInvitationResult> {
    return this.request("POST", "/api/v1/hosts/invitations", { invited_email: invitedEmail, membership_role: membershipRole });
  }

  acceptInvitation(invitationToken: string): Promise<AcceptInvitationResult> {
    return this.request("POST", "/api/v1/hosts/invitations/accept", { invitation_token: invitationToken });
  }

  // Mass upload (Phase 3)

  initiateUploadBatch(input: { total_files: number; total_bytes: number; files: Array<{ client_file_token: string; original_filename?: string; expected_bytes: number }> }): Promise<UploadBatchResult> {
    return this.request("POST", "/api/v1/storage/upload-batches/initiate", input);
  }

  initiateMultipart(uploadBatchId: string, clientFileToken: string): Promise<MultipartInitiateResult> {
    return this.request("POST", "/api/v1/storage/multipart/initiate", { upload_batch_id: uploadBatchId, client_file_token: clientFileToken });
  }

  completeMultipart(uploadItemId: string, parts: Array<{ part_number: number; etag: string }>): Promise<MultipartCompleteResult> {
    return this.request("POST", "/api/v1/storage/multipart/complete", { upload_item_id: uploadItemId, parts });
  }

  finalizeBatch(uploadBatchId: string): Promise<BatchFinalizeResult> {
    return this.request("POST", `/api/v1/storage/upload-batches/${uploadBatchId}/finalize`);
  }
}
