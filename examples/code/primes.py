import itertools


def primes():
    """Yields the prime numbers, using the sieve of Eratosthenes."""
    composites = {}
    for n in itertools.count(2):
        if n not in composites:
            yield n
            composites[n * n] = [n]
            continue
        for p in composites.pop(n):
            composites.setdefault(n + p, []).append(p)


print(list(itertools.islice(primes(), 20)))
