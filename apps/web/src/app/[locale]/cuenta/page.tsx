import { getTranslations } from "next-intl/server";
import { auth, signIn, signOut } from "@/auth";
import { Panel } from "@questlog/ui";

export default async function CuentaPage() {
  const session = await auth();
  const t = await getTranslations("cuenta");

  if (!session?.user) {
    return (
      <main className="mx-auto max-w-2xl p-10">
        <Panel>
          <p style={{ marginBottom: 16 }}>{t("signedOut")}</p>
          <form
            action={async () => {
              "use server";
              await signIn("keycloak");
            }}
          >
            <button
              type="submit"
              style={{
                background: "var(--ql-accent)",
                color: "var(--ql-accent-ink)",
                padding: "10px 18px",
                border: "none",
              }}
            >
              {t("signIn")}
            </button>
          </form>
        </Panel>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-2xl p-10">
      <Panel variant="signature">
        <p>
          {t("signedInAs", {
            name: session.user.name ?? session.user.email ?? "",
          })}
        </p>
        <p style={{ color: "var(--ql-muted)", marginTop: 4 }}>
          {t("roles", {
            roles: session.roles?.join(", ") || t("noRoles"),
          })}
        </p>
        <form
          action={async () => {
            "use server";
            await signOut();
          }}
          style={{ marginTop: 16 }}
        >
          <button
            type="submit"
            style={{
              background: "transparent",
              color: "var(--ql-text)",
              border: "1px solid var(--ql-frame-dim)",
              padding: "10px 18px",
            }}
          >
            {t("signOut")}
          </button>
        </form>
      </Panel>
    </main>
  );
}
