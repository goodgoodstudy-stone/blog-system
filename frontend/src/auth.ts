import { useQuery } from "@tanstack/react-query";
import { api, type User } from "./api";

export function useCurrentUser() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api<User>("/auth/me"),
    staleTime: 0,
  });
}
