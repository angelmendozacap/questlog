import { getTranslations } from "next-intl/server";
import { Panel } from "@questlog/ui";
import { auth } from "@/auth";

export default async function Home() {
  // Defense-in-depth, not redundancy: Next.js's App Router always invokes
  // this page component to build the `children` prop it hands to the
  // layout, even on requests where the layout's role gate ultimately
  // discards that prop and renders <AccessGate> instead. A discarded React
  // element still gets serialized into the RSC flight payload as an
  // unreferenced chunk — verified by inspecting a real HTTP response body
  // for a non-admin session: this page's rendered markup was present as
  // inert JSON in a <script> tag despite never being mounted or visible.
  // Gating here too means the unauthorized branch renders `null`, so
  // there's nothing but an empty chunk to ship. Every future portal page
  // under this layout needs the same check for the same reason — the
  // layout's gate alone controls what's *displayed*, not what's *sent*.
  const session = await auth();
  const isAdmin = session?.roles?.includes("admin") ?? false;
  if (!isAdmin) return null;

  const t = await getTranslations("home");

  return (
    <main className="mx-auto max-w-2xl p-10">
      <Panel variant="signature">
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <p style={{ color: "var(--ql-muted)" }}>{t("tagline")}</p>
      </Panel>
    </main>
  );
}
