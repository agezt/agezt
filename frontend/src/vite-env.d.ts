/// <reference types="vite/client" />

// Fontsource packages are imported purely for their @font-face side effects and
// ship no type declarations; TS 6 requires a declaration for side-effect imports.
declare module "@fontsource-variable/inter";
