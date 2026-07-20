import { getTranslations } from "next-intl/server";
import { Panel } from "@questlog/ui";

export default async function Home() {
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
