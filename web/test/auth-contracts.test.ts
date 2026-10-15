import { describe, expect, it } from "vitest";
import {
  AUTH_ERROR_CODES,
  CAPABILITY_CODES,
  HOST_APPLICATION_STATUSES,
  HOST_DOCUMENT_TYPES,
  HOST_MEMBERSHIP_ROLES,
  isAuthErrorCode,
  isCapabilityCode,
} from "../src/types/api.js";

describe("Domain 01 capability taxonomy", () => {
  it("matches the backend capability set exactly", () => {
    expect([...CAPABILITY_CODES].sort()).toEqual(
      ["BUYER", "CREATOR", "HOST_ADMIN", "HOST_LEADER", "SUPER_ADMIN"].sort(),
    );
  });

  it("narrows unknown capability strings", () => {
    expect(isCapabilityCode("HOST_LEADER")).toBe(true);
    expect(isCapabilityCode("host_leader")).toBe(false);
    expect(isCapabilityCode("ROOT")).toBe(false);
  });
});

describe("Domain 01 error catalog", () => {
  it("carries the documented authentication and authorization codes", () => {
    for (const code of [
      "AUTH_UNAUTHORIZED",
      "AUTH_FORBIDDEN",
      "APPLICATION_ALREADY_EXISTS",
      "ADMIN_LIMIT_EXCEEDED",
      "INVITATION_EXPIRED",
      "TOKEN_REUSE_DETECTED",
    ]) {
      expect(AUTH_ERROR_CODES).toContain(code);
      expect(isAuthErrorCode(code)).toBe(true);
    }
  });

  it("rejects codes outside the catalog", () => {
    expect(isAuthErrorCode("SOMETHING_ELSE")).toBe(false);
  });
});

describe("Domain 01 Host enums", () => {
  it("keeps membership roles aligned with the backend constants", () => {
    expect([...HOST_MEMBERSHIP_ROLES]).toEqual(["LEADER", "ADMIN"]);
  });

  it("keeps application statuses aligned with the backend constants", () => {
    expect([...HOST_APPLICATION_STATUSES]).toEqual(["SUBMITTED", "APPROVED", "REJECTED"]);
  });

  it("keeps legal document types aligned with the backend constants", () => {
    expect([...HOST_DOCUMENT_TYPES]).toEqual(["KTP_DIREKSI", "NPWP_BADAN", "NIB_OSS", "AKTA_PENDIRIAN"]);
  });
});
