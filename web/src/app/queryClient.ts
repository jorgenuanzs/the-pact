import { QueryClient } from "@tanstack/react-query";

import { APIError } from "@/api/client";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2_000,
      retry: (attempt, error) => !(error instanceof APIError && error.status < 500) && attempt < 2,
      refetchOnWindowFocus: true,
    },
    mutations: {
      retry: false,
    },
  },
});
