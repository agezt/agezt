import { useAdvanced } from "@/lib/advanced";

// <Advanced> renders its children only when global Advanced mode is on. Use it to
// fold text-heavy / diagnostic detail away from the calm default view — NOT for
// metrics or charts (those stay visible; the console should feel alive). Pair a
// calm summary outside it with the dense detail inside:
//
//   <p>3 providers connected.</p>
//   <Advanced><RawProviderTable …/></Advanced>
export function Advanced({ children }: { children: React.ReactNode }) {
  const { advanced } = useAdvanced();
  return advanced ? <>{children}</> : null;
}

// <Calm> is the inverse: shown only when Advanced mode is OFF. Handy for a terse
// "open Advanced for details" hint that disappears once details are showing.
export function Calm({ children }: { children: React.ReactNode }) {
  const { advanced } = useAdvanced();
  return advanced ? null : <>{children}</>;
}
