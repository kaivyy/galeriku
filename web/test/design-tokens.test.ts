import { describe, it, expect } from 'vitest';
import { AntislopDials, BrandPalette, LayoutMetrics, validateAntislopCopy } from '../src/tokens/design-system.js';

describe('Antislop Design System & Craftsmanship Compliance', () => {
  it('enforces explicitly declared antislop dials', () => {
    expect(AntislopDials.ENERGY).toBe(2); // Balanced
    expect(AntislopDials.RHYTHM).toBe(2); // Content-driven variation
    expect(AntislopDials.MOTION).toBe(1); // Essential micro-interactions
  });

  it('guarantees minimum tap target metric of 44px for touch interfaces (R-03)', () => {
    expect(LayoutMetrics.minTapTargetPx).toBeGreaterThanOrEqual(44);
  });

  it('limits core palette to max 3 primary + 1 accent color (R-29)', () => {
    expect(BrandPalette.coreColors.length).toBeLessThanOrEqual(3);
    expect(BrandPalette.accentColor).toBeDefined();
  });

  it('rejects copy containing em dash (—) under R-02 Hard Gate', () => {
    const invalidCopy = "Temukan fotomu — cepat dan mudah";
    expect(() => validateAntislopCopy(invalidCopy)).toThrowError(/R-02 Violation: Em dash/);

    const validCopy = "Temukan fotomu: cepat dan mudah.";
    expect(validateAntislopCopy(validCopy)).toBe(true);
  });
});
