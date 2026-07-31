import type { ReactNode } from "react";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";
import { routing } from "@/i18n/routing";
import { auth, signIn, signOut } from "@/auth";
import "@questlog/ui/tokens.css";
import "../globals.css";

export const metadata = {
  title: "QuestLog Admin",
  description: "Moderación, usuarios y curación de catálogo.",
};

export default async function LocaleLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  if (!hasLocale(routing.locales, locale)) notFound();

  const session = await auth();
  const t = await getTranslations("admin");
  const isAdmin = session?.roles?.includes("admin") ?? false;

  return (
    <html lang={locale} data-admin="true">
      <body>
        <NextIntlClientProvider>
          {!session?.user ? (
            <AccessGate
              title={t("signInTitle")}
              message={t("signInMessage")}
              action={{ label: t("signIn"), kind: "signIn" }}
            />
          ) : !isAdmin ? (
            <AccessGate
              title={t("deniedTitle")}
              message={t("deniedMessage")}
              action={{ label: t("signOut"), kind: "signOut" }}
            />
          ) : (
            children
          )}
        </NextIntlClientProvider>
      </body>
    </html>
  );
}

function AccessGate({
  title,
  message,
  action,
}: {
  title: string;
  message: string;
  action: { label: string; kind: "signIn" | "signOut" };
}) {
  return (
    <main
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        padding: 40,
      }}
    >
      <div
        style={{
          background: "var(--ql-panel)",
          border: "1px solid var(--ql-frame-dim)",
          padding: "28px 32px",
          maxWidth: 420,
          textAlign: "center",
        }}
      >
        <h1 style={{ fontSize: 20, marginBottom: 10 }}>{title}</h1>
        <p style={{ color: "var(--ql-muted)", marginBottom: 20 }}>{message}</p>
        <form
          action={async () => {
            "use server";
            if (action.kind === "signIn") {
              await signIn("keycloak");
            } else {
              await signOut();
            }
          }}
        >
          <button
            type="submit"
            style={{
              background: "var(--ql-accent)",
              color: "var(--ql-accent-ink)",
              border: "none",
              padding: "10px 20px",
            }}
          >
            {action.label}
          </button>
        </form>
      </div>
    </main>
  );
}
