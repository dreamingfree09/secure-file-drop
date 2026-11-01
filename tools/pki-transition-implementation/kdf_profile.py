"""Public-input KDF calibration; no Owner credential or private key is accessed."""
import hashlib,math,secrets,statistics,time
MIN_ITERATIONS=600000
MAX_ITERATIONS=3000000
TARGET_MS=250
def calibrate():
    seed=secrets.token_bytes(32);salt=secrets.token_bytes(16)
    timings=[]
    for _ in range(3):
        start=time.perf_counter()
        hashlib.pbkdf2_hmac("sha256",seed,salt,MIN_ITERATIONS)
        timings.append((time.perf_counter()-start)*1000)
    median=statistics.median(timings)
    selected=max(MIN_ITERATIONS,min(MAX_ITERATIONS,int(math.ceil(MIN_ITERATIONS*TARGET_MS/max(median,0.01)/100000))*100000))
    return {"scope":"public/random synthetic-input derivation benchmark, not a password hash",
            "algorithm":"PBKDF2-HMAC-SHA256","samples_ms":[round(x,3) for x in timings],
            "median_ms":round(median,3),"target_ms":TARGET_MS,"minimum_iterations":MIN_ITERATIONS,
            "iterations":selected,"max_iterations":MAX_ITERATIONS,
            "exact_PKCS8_provider_latency_qualified":False}
