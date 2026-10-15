/**
 * Domain 01 (Identity, Auth and Host B2B) contract types.
 * Hand-maintained mirror of GALERIKU_OPENAPI_v1_DRAFT.yaml and the backend
 * constants in backend/internal/domain. Keep the literal unions below in sync
 * with the Go constants; web/test/auth-contracts.test.ts fails when they drift.
 */

export const CAPABILITY_CODES = [
  "BUYER",
  "CREATOR",
  "HOST_ADMIN",
  "HOST_LEADER",
  "SUPER_ADMIN",
] as const;
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

export const HOST_DOCUMENT_TYPES = [
  "KTP_DIREKSI",
  "NPWP_BADAN",
  "NIB_OSS",
  "AKTA_PENDIRIAN",
] as const;
export type HostDocumentType = (typeof HOST_DOCUMENT_TYPES)[number];

export const INVITATION_STATUSES = ["PENDING", "ACCEPTED", "REVOKED", "EXPIRED"] as const;
export type InvitationStatus = (typeof INVITATION_STATUSES)[number];

/** Error codes emitted by the Domain 01 endpoints. */
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
  "NOT_FOUND",
] as const;
export type AuthErrorCode = (typeof AUTH_ERROR_CODES)[number];

export function isAuthErrorCode(value: string): value is AuthErrorCode {
  return (AUTH_ERROR_CODES as readonly string[]).includes(value);
}

/** Standard success envelope (GALERIKU_API_ARCHITECTURE_SPEC.md section 4.1). */
export interface SuccessEnvelope<T> {
  success: true;
  data: T;
  meta: {
    timestamp?: string;
    request_id: string;
    next_cursor?: string | null;
  };
}

/** Standard error envelope (GALERIKU_API_ARCHITECTURE_SPEC.md section 4.2). */
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
