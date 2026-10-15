/**
 * Full API contract types for Galeriku, Domains 01 through 07.
 * Mirror of GALERIKU_OPENAPI_v1_DRAFT.yaml and the Go domain constants.
 * Drift is caught by web/test/contracts.test.ts.
 */

// Domain 01: Identity, Auth & Host B2B

export const CAPABILITY_CODES = ["BUYER", "CREATOR", "HOST_ADMIN", "HOST_LEADER", "SUPER_ADMIN"] as const;
export type CapabilityCode = (typeof CAPABILITY_CODES)[number];
export function isCapabilityCode(value: string): value is CapabilityCode {
  return (CAPABILITY_CODES as readonly string[]).includes(value);
}

export const HOST_MEMBERSHIP_ROLES = ["LEADER", "ADMIN"] as const;
export type HostMembershipRole = (typeof HOST_MEMBERSHIP_ROLES)[number];

export const HOST_MEMBERSHIP_STATUSES = ["ACTIVE", "INACTIVE", "REVOKED"] as const;
export type HostMembershipStatus = (typeof HOST_MEMBERSHIP_STATUSES)[number];

export const HOST_APPLICATION_STATUSES = ["SUBMITTED", "APPROVED", "REJECTED"] as const;
export type HostApplicationStatus = (typeof HOST_APPLICATION_STATUSES)[number];

export const HOST_DOCUMENT_TYPES = ["KTP_DIREKSI", "NPWP_BADAN", "NIB_OSS", "AKTA_PENDIRIAN"] as const;
export type HostDocumentType = (typeof HOST_DOCUMENT_TYPES)[number];

export const INVITATION_STATUSES = ["PENDING", "ACCEPTED", "REVOKED", "EXPIRED"] as const;
export type InvitationStatus = (typeof INVITATION_STATUSES)[number];

// Domain 02: Events & FotoTree

export const EVENT_STATUSES = ["DRAFT", "PUBLISHED", "ARCHIVED"] as const;
export type EventStatus = (typeof EVENT_STATUSES)[number];

export const SESSION_STATUSES = ["SCHEDULED", "ACTIVE", "COMPLETED"] as const;
export type SessionStatus = (typeof SESSION_STATUSES)[number];

export const LOCATION_STATUSES = ["ACTIVE", "INACTIVE"] as const;
export type LocationStatus = (typeof LOCATION_STATUSES)[number];

// Domain 03: Storage & Upload

export const UPLOAD_BATCH_STATUSES = ["RESERVED", "COMPLETED", "EXPIRED"] as const;
export type UploadBatchStatus = (typeof UPLOAD_BATCH_STATUSES)[number];

export const UPLOAD_ITEM_STATUSES = ["PENDING", "UPLOADED"] as const;
export type UploadItemStatus = (typeof UPLOAD_ITEM_STATUSES)[number];

export const MAX_FILES_PER_BATCH = 2000;

// Domain 04: Cart, Orders & Checkout

export const ORDER_STATUSES = ["PENDING_PAYMENT", "PAID", "CANCELLED"] as const;
export type OrderStatus = (typeof ORDER_STATUSES)[number];

export const CART_STATUSES = ["ACTIVE", "CONVERTED"] as const;
export type CartStatus = (typeof CART_STATUSES)[number];

// Domain 05: Payments & Fulfillment

export const PAYMENT_STATUSES = ["PENDING", "SETTLED", "FAILED", "EXPIRED"] as const;
export type PaymentStatus = (typeof PAYMENT_STATUSES)[number];

export const FULFILLMENT_STATUSES = ["WAITING_MASTER", "FULFILLED", "SLA_BREACH"] as const;
export type FulfillmentStatus = (typeof FULFILLMENT_STATUSES)[number];

export const DOWNLOAD_GRANT_TYPES = ["NON_HIRES_DOWNLOAD", "HIRES_DOWNLOAD"] as const;
export type DownloadGrantType = (typeof DOWNLOAD_GRANT_TYPES)[number];

// Domain 06: Finance

export const LEDGER_DIRECTIONS = ["DEBIT", "CREDIT"] as const;
export type LedgerDirection = (typeof LEDGER_DIRECTIONS)[number];

export const SETTLEMENT_STATUSES = ["HOLDING", "RELEASED"] as const;
export type SettlementStatus = (typeof SETTLEMENT_STATUSES)[number];

// Error catalog across all domains

export const AUTH_ERROR_CODES = [
  "AUTH_UNAUTHORIZED",
  "AUTH_FORBIDDEN",
  "INVALID_INPUT",
  "INVALID_CREDENTIALS",
  "ACCOUNT_CONFLICT",
  "TOKEN_EXPIRED",
  "TOKEN_REUSE_DETECTED",
  "APPLICATION_ALREADY_EXISTS",
  "ADMIN_LIMIT_EXCEEDED",
  "INVITATION_EXPIRED",
  "INVITATION_CONSUMED",
  "LAST_LEADER_PROTECTED",
  "APPLICATION_ALREADY_REVIEWED",
  "MEMBERSHIP_ALREADY_EXISTS",
  "SLUG_ALREADY_EXISTS",
  "QUOTA_EXCEEDED",
  "ORDER_NOT_PAYABLE",
  "DOWNLOAD_NOT_PERMITTED",
  "BATCH_NOT_FINALIZABLE",
  "IDEMPOTENCY_KEY_CONFLICT",
  "NOT_FOUND",
] as const;
export type AuthErrorCode = (typeof AUTH_ERROR_CODES)[number];

export function isAuthErrorCode(value: string): value is AuthErrorCode {
  return (AUTH_ERROR_CODES as readonly string[]).includes(value);
}

// Standard envelopes

export interface SuccessEnvelope<T> {
  success: true;
  data: T;
  meta: {
    timestamp?: string;
    request_id: string;
    next_cursor?: string | null;
  };
}

export interface ErrorEnvelope {
  success: false;
  error: {
    code: AuthErrorCode | string;
    message: string;
    details?: unknown;
    request_id?: string;
  };
  meta: {
    timestamp: string;
    request_id: string;
  };
}

// Response shapes

export interface HostApplicationResult {
  application_id: string;
  status: HostApplicationStatus;
  submitted_at?: string;
}

export interface MyHostApplication {
  application_id: string;
  status: HostApplicationStatus;
  legal_name: string;
  display_name: string;
  documents_uploaded: number;
}

export interface DocumentUploadResult {
  document_id: string;
  object_key: string;
  upload_url: string;
  mime_type: string;
  expires_in_seconds: number;
}

export interface DocumentViewResult {
  document_id: string;
  view_url: string;
  expires_in_seconds: number;
}

export interface HostOrganizationSummary {
  id: string;
  legal_name: string;
  display_name: string;
  business_field: string;
  verification_status: HostApplicationStatus;
  max_admin_count: number;
  current_admin_count: number;
}

export interface HostMember {
  membership_id: string;
  user_id: string;
  display_name: string;
  role: HostMembershipRole;
  status: HostMembershipStatus;
  created_at: string;
  revoked_at?: string;
}

export interface HostBankAccount {
  id: string;
  bank_code: string;
  account_number_masked: string;
  account_holder_name: string;
  verification_status: string;
  created_at: string;
}

export interface HostInvitationResult {
  invitation_id: string;
  status: InvitationStatus;
  expires_at: string;
  invitation_token: string;
}

export interface AcceptInvitationResult {
  status: HostMembershipStatus;
  role: HostMembershipRole;
  organization_id: string;
}

export interface UploadBatchResult {
  upload_batch_id: string;
  status: UploadBatchStatus;
  reserved_bytes: number;
  reservation_expires_at?: string;
}

export interface MultipartInitiateResult {
  upload_item_id: string;
  provider_upload_id: string;
  s3_object_key: string;
}

export interface MultipartCompleteResult {
  upload_item_id: string;
  status: string;
  object_key: string;
  actual_bytes: number;
}

export interface BatchFinalizeResult {
  upload_batch_id: string;
  status: string;
  total_files_uploaded: number;
  actual_bytes: number;
  quota_reconciled: boolean;
  event_published: boolean;
}

export interface BIBSearchResult {
  bib_normalized: string;
  total_photos: number;
  photos: Array<{
    photo_id: string;
    price_idr: number;
    bib_checkout_requires_verification: boolean;
  }>;
}

export interface CartResult {
  cart_id: string;
  total_items: number;
  total_amount_idr: number;
  items: Array<{
    cart_item_id: string;
    photo_id: string;
    price_idr: number;
  }>;
}

export interface PaymentInitiationResult {
  payment_id: string;
  provider: string;
  qris_string?: string;
  qris_image_url?: string;
  amount_idr: number;
  expires_at: string;
}

export interface DownloadGrantResult {
  download_token: string;
  asset_type: DownloadGrantType;
  watermark_applied?: string;
  download_url: string;
  expires_at: string;
}

export interface UploadMasterResult {
  fulfillment_status: FulfillmentStatus;
  fulfilled_at?: string;
  hires_download_ready: boolean;
}

export interface RevenueSplit {
  platform_idr: number;
  host_idr: number;
  creator_idr: number;
}

export interface RefundResult {
  refund_item_id: string;
  order_item_id: string;
  eligible_amount_idr: number;
  buyer_credit_amount_idr: number;
  creator_clawback_idr: number;
  host_clawback_idr: number;
  platform_clawback_idr: number;
  status: string;
}

export interface WalletBalanceResult {
  available_balance_idr: number;
  pending_hold_balance_idr: number;
}
