// Side-effect imports for self-hosted variable fonts loaded via Vite.
// These packages ship only CSS files and have no TS types — the import
// tells Vite to emit the woff2 bundle under the strict CSP (font-src 'self').
declare module "@fontsource-variable/jetbrains-mono";
declare module "@fontsource-variable/space-grotesk";
