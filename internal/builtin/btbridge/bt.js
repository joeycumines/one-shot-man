export const running = "running";
export const success = "success";
export const failure = "failure";

export const node =
  (tick, ...children) =>
  () => [tick, children];

export function tick(node) {
  const [tick, children] = node();
  return tick(children);
}

/**
 * A tick implementation which ticks each child sequentially, until the first
 * error (bubbles), the first non-success status (returning the status), or all
 * children are ticked (returning success).
 *
 * This is a stateless tick implementation.
 */
export async function sequence(children) {
  for (const child of children) {
    const status = await tick(child);
    if (status === running) {
      return running;
    }
    if (status !== success) {
      return failure;
    }
  }
  return success;
}

/**
 * A tick implementation which ticks each child sequentially, until the first
 * error (bubbles), the first non-failure status (returning the status), or all
 * children are ticked (returning failure).
 *
 * This is a stateless tick implementation.
 */
export async function fallback(children) {
  for (const child of children) {
    const status = await tick(child);
    if (status === running) {
      return running;
    }
    if (status === success) {
      return success;
    }
  }
  return failure;
}

/**
 * Run children, in parallel, returning {@link success} if at least
 * `successThreshold` children succeed, {@link failure} if at least
 * `children.length - successThreshold` children fail, or otherwise
 * {@link running}, if neither condition is met. For child ticks which may
 * return {@link running}, consider if {@link memorize} is appropriate -
 * wrapping {@link parallel} and/or child tick(s).
 *
 * This is a stateless tick implementation.
 *
 * See also {@link fork}, a similar implementation to
 * `bt.memorize(bt.parallel())`, but which won't return failure until all
 * children have returned a non-running status.
 *
 * @param [successThreshold] Defaults to the number of children.
 * @returns {*}
 */
export const parallel = (successThreshold) =>
  async function parallel(children) {
    const statuses = await Promise.all(children.map(tick));

    let successCount = 0;
    let failureCount = 0;
    for (let status of statuses) {
      if (status === success) {
        successCount++;
      } else if (status === failure) {
        failureCount++;
      }
    }

    const threshold = successThreshold ?? statuses.length;

    if (successCount >= threshold) {
      return success;
    } else if (failureCount > statuses.length - threshold) {
      return failure;
    }

    return running;
  };

/**
 * Memorize encapsulates a tick, caching the first non-{@link running}
 * status for each child, per "execution", defined as the period until the
 * first non-running status, of the encapsulated tick. This allows stateless
 * implementations, such as {@link sequence}, {@link selector}, and
 * {@link parallel}, to only run each child once, per execution. When used with
 * a deterministic tick w/o side effects, the returned tick will "resume", just
 * after the last non-running status.
 *
 * This is a stateful tick wrapper.
 *
 * @param tick
 * @returns {function(*): Promise<*>}
 */
export const memorize = (tick) => {
  let nodes;
  return async function memorize(children) {
    if (!nodes) {
      nodes = children.map((child) => {
        let override;
        return () => {
          const [tick, nodes] = child();
          if (override) {
            return [override, nodes];
          }
          return [
            async (children) => {
              const status = await tick(children);
              if (status !== running) {
                override = newOverrideTick(status);
              }
              return status;
            },
            nodes,
          ];
        };
      });
    }
    const status = await tick(nodes);
    if (status !== running) {
      nodes = undefined;
    }
    return status;
  };
};

/**
 * Async wraps a tick such that it will return running until it settles.
 *
 * This is a stateful tick wrapper.
 */
export const async = (tick) => {
  let promise;
  let settled = false;
  return async function async(children) {
    if (!promise) {
      promise = tick(children).finally(() => {
        settled = true;
      });
      // avoid unhandled promise rejection
      promise.catch(noop);
    }
    if (!settled) {
      return running;
    }
    const v = promise;
    promise = undefined;
    settled = false;
    return v;
  };
};

/**
 * Inverts the result of a tick.
 *
 * This is a stateless tick wrapper.
 */
export const not = (tick) =>
  async function not(children) {
    const status = await tick(children);
    switch (status) {
      case running:
        return running;
      case failure:
        return success;
      default:
        return failure;
    }
  };

/**
 * Fork will tick all children at once, returning after all children did so,
 * returning {@link running} if any children were running, ticking those
 * children on subsequent ticks, in a cycle. Once the cycle is complete,
 * {@link success} will be returned, if all children succeeded, otherwise
 * {@link failure}.
 *
 * WARNING: Errors will bubble immediately. TODO: Consider alternate handling.
 *
 * This is a stateful tick implementation.
 *
 * See also {@link parallel}, which may be used in a similar manner, but has
 * distinct use cases.
 */
export const fork = () => {
  let status;
  let remaining;
  return async function fork(children) {
    if (status === undefined) {
      // cycle start
      remaining = children.slice();
      status = success;
    }

    const statuses = await Promise.all(remaining.map(tick));

    remaining = remaining.filter((_, i) => {
      const v = statuses[i];
      if (v === running) {
        return true;
      }
      if (v !== success) {
        status = failure;
      }
      return false;
    });

    if (remaining.length === 0) {
      // cycle end
      const v = status;
      status = undefined;
      return v;
    }

    return running;
  };
};

/**
 * Returns a tick that will return `readyStatus` at most once per `intervalMs`,
 * otherwise returning `pendingStatus`. The first tick will always return
 * `readyStatus`, unless the optional `initialReady` parameter is false.
 *
 * Returning {@link running} on pending (the default) is convenient to avoid
 * "aborting" memorized sequences or similar, if the interval has not yet
 * elapsed.
 */
export const interval = (
  intervalMs,
  initialReady = true,
  pendingStatus = running,
  readyStatus = success,
) => {
  let last = initialReady ? undefined : Date.now();
  return async function interval() {
    const now = Date.now();
    if (last !== undefined && now - last < intervalMs) {
      return pendingStatus;
    }
    last = now;
    return readyStatus;
  };
};

/**
 * Runs a node tree, by ticking it on each iteration of the ticker.
 *
 * WARNING: The `return` method of the ticker will always be called, e.g. if
 * an abort signal is provided, even if the ticker was an existing iterator.
 *
 * @param node
 * @param ticker {AsyncIterable|AsyncIterator}
 * @param abort {AbortSignal|undefined}
 * @returns {Promise<void>}
 */
export async function run(node, ticker, abort) {
  const iterable =
    Symbol.asyncIterator in ticker ? ticker[Symbol.asyncIterator]() : ticker;
  try {
    // eslint-disable-next-line no-constant-condition
    while (true) {
      const next = iterable.next();
      let promise = next;

      // WARNING: Using single promise outside the loop + Promise.race is not
      // a viable alternative, as such a promise would retain a reference to
      // callbacks from each iteration, and thus leak memory.
      if (abort) {
        abort.throwIfAborted();
        let listener;
        promise = new Promise((resolve, reject) => {
          listener = () => {
            reject(abort.reason);
          };
          abort.addEventListener("abort", listener);
          next.then(resolve, reject);
        }).finally(() => {
          abort.removeEventListener("abort", listener);
        });
      }

      const { done } = await promise;
      if (done) {
        break;
      }

      await tick(node);
    }
  } finally {
    await iterable.return();
  }
}

const newOverrideTick = (status) => async () => status;

const noop = () => {};
