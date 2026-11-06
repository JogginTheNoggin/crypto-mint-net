# How to grind (CPU and GPU)

## Summary

Grinding is searching for layers: proof of work that adds weight to an
entry and so makes the ledger harder to overturn. Your CPU and any GPU
can grind. **Work** shows your devices, runs jobs, and schedules jobs the
node runs by itself.

## See your devices

**Work**, **Devices** lists each device: `cpu`, and each graphics card as
`gpu:0`, `gpu:1` and so on. For each, it shows the hashes it grinds fast,
which hashes **auto** picks it for, and a note if it cannot grind (for
example a missing driver).

### Getting a GPU to show

- A GPU grinds through OpenCL. Install your card's normal graphics driver
  with OpenCL support (NVIDIA, AMD and Intel drivers include it; on Linux
  you may need the vendor's OpenCL package).
- Restart the node after installing a driver, then press **Refresh** on
  **Devices**.
- A GPU grinds the SHA-256 family of hashes. For other hashes, **auto**
  uses the CPU.
- If a GPU causes trouble, start the program with the environment
  variable `POON_OPENCL=off` to use only the CPU.

## Grind now and watch it

1. **Work**, **Run a job**. **Target** says what to grind:
   - `tip`: layers on each pool's newest entry (helps secure the ledger);
   - `pending`: build pending transactions into new entries;
   - `tx:TID`: work until that transaction is in the ledger;
   - or an entry hash.
2. **Device**: leave it on `auto` to let the node pick the fastest for
   each hash, or choose `cpu` or `gpu:0`.
3. **For**: how long, for example `10m`. Leave empty to run until you stop.
4. Press **Run and follow**. You see the plan the node picked (device and
   grinder), the rate, and each layer or entry found. **Stop** ends it.

You can narrow a job to a **Series id**, **Pool** or **Mint serial**, set
**Threads** for the CPU, and stop once the weight behind a transaction
reaches a number (**Until from reaches**).

## Let the node grind in the background

- **Start a job in the background**: the same job, run by the node. It
  shows under **Jobs** on **Work**, with its rate.
- **Stop a job**: by **Job id**, or **Every job**.

## Grind on a schedule

**Schedule a job** adds a job the node runs by itself, at the hours you
give in **Only in hours** (for example overnight). **Schedule** lists
them, each with **Remove**. **Schedule defaults** sets the device and
threads every job uses unless it names its own, and the layer floor.

## How fast is it?

**Benchmark** measures each device on each hash for a few seconds,
without a node. Use it to compare CPU and GPU on your machine.

To be paid for grinding, see [How to use the bounty market](5-how-to-use-the-bounty-market.md).

In the terminal: [How to grind (headless)](../headless/5-how-to-grind.md).
