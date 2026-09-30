import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type DependencyList,
  type Dispatch,
  type SetStateAction,
} from "react";

export type AsyncFetcher<T> = (signal: AbortSignal) => Promise<T>;

interface AsyncResourceOptions<T> {
  initial: T;
  pollMs?: number;
  errorMessage?: string;
}

export interface AsyncResource<T> {
  data: T;
  setData: Dispatch<SetStateAction<T>>;
  loading: boolean;
  error: string;
}

export function useAsyncResource<T>(
  fetcher: AsyncFetcher<T> | null,
  deps: DependencyList,
  { initial, pollMs, errorMessage = "Request failed" }: AsyncResourceOptions<T>,
): AsyncResource<T> {
  const [data, setData] = useState<T>(initial);
  const [loading, setLoading] = useState(fetcher !== null);
  const [error, setError] = useState("");
  const fetcherRef = useRef(fetcher);
  const initialRef = useRef(initial);
  const errorMessageRef = useRef(errorMessage);

  useLayoutEffect(() => {
    fetcherRef.current = fetcher;
    errorMessageRef.current = errorMessage;
  });

  useEffect(() => {
    if (!fetcherRef.current) {
      setData(initialRef.current);
      setLoading(false);
      setError("");
      return;
    }

    let controller = new AbortController();
    const run = () => {
      const fetchNow = fetcherRef.current;
      if (!fetchNow) return;
      controller.abort();
      const current = new AbortController();
      controller = current;
      fetchNow(current.signal)
        .then((result) => {
          if (current.signal.aborted) return;
          setData(result);
          setError("");
        })
        .catch((err: unknown) => {
          if (current.signal.aborted) return;
          setError(
            err instanceof Error ? err.message : errorMessageRef.current,
          );
        })
        .finally(() => {
          if (!current.signal.aborted) setLoading(false);
        });
    };

    setLoading(true);
    setError("");
    run();
    const interval = pollMs ? setInterval(run, pollMs) : undefined;
    return () => {
      controller.abort();
      clearInterval(interval);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, pollMs]);

  return { data, setData, loading, error };
}
