import { useQuery } from "@tanstack/react-query";
import { blogRequest, type User } from "./blog-client";

export function useCurrentUser() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => blogRequest<User>("/auth/me"),
    staleTime: 0,
  });
}
