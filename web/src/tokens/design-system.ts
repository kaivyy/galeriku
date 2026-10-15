/**
 * Galeriku Antislop Design System Tokens
 * Conforming strictly to antislop.md core filter & UI_UX_SPEC.md
 */

export const AntislopDials = {
  ENERGY: 2 as const, // 1: Calm, 2: Balanced, 3: Bold
  RHYTHM: 2 as const, // 1: Uniform, 2: Content-driven, 3: Mixed
  MOTION: 1 as const, // 1: Hover/micro, 2: Scroll transitions, 3: Choreographed
};

export const BrandPalette = {
  coreColors: [
    '#0F172A', // Deep Slate (Primary Background & Contrast Text)
    '#4F46E5', // Electric Indigo (Brand Primary Action)
    '#FFFFFF', // Pure White (Surface Card)
  ],
  neutralColors: {
    gray50: '#F8FAFC',
    gray100: '#F1F5F9',
    gray500: '#64748B',
    gray900: '#0F172A',
  },
  accentColor: '#F59E0B', // Amber (Attention & Status Badges)
};

export const LayoutMetrics = {
  minTapTargetPx: 44, // Minimum mobile interactive touch dimension (R-03)
  standardRadiusPx: 8, // Non-pill geometric border radius (R-11)
  contentMaxWidthPx: 1280,
};

/**
 * Validates UI copy against Antislop Hard Gate R-02 (forbidden em dash).
 */
export function validateAntislopCopy(text: string): boolean {
  if (text.includes('—')) {
    throw new Error('R-02 Violation: Em dash character (—) is strictly forbidden in UI copy. Use comma, period, colon, or parentheses.');
  }
  return true;
}
