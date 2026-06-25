import type {
  Profile,
  CreateProfileRequest,
  CreateProfileResponse,
} from "../../generated/types";
import { request } from "./client";

// Profiles — endpoint is /profiles (no /api prefix)
export async function fetchProfiles(): Promise<Profile[]> {
  return request<Profile[]>("/profiles");
}

export async function createProfile(
  data: CreateProfileRequest,
): Promise<CreateProfileResponse> {
  return request<CreateProfileResponse>("/profiles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteProfile(id: string): Promise<void> {
  await request<void>(`/profiles/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export interface UpdateProfileRequest {
  name?: string;
  useWhen?: string;
  description?: string;
}

export interface UpdateProfileResponse {
  status: string;
  id: string;
  name: string;
}

export async function updateProfile(
  id: string,
  data: UpdateProfileRequest,
): Promise<UpdateProfileResponse> {
  return request<UpdateProfileResponse>(`/profiles/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}
