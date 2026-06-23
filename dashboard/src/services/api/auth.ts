import { request } from "./client";

export async function login(token: string): Promise<void> {
  await request<{ status: string }>(
    "/api/auth/login",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    },
    {
      suppressAuthRedirect: true,
    },
  );
}

export async function logout(): Promise<void> {
  await request<{ status: string }>(
    "/api/auth/logout",
    {
      method: "POST",
    },
    {
      suppressAuthRedirect: true,
    },
  );
}

export async function elevate(token: string): Promise<void> {
  await request<{ status: string }>(
    "/api/auth/elevate",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    },
    {
      suppressAuthRedirect: true,
    },
  );
}
