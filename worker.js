// This process is a long-lived worker: main.go keeps a pool of these alive
// and feeds each one jobs, one at a time, over stdin as a single JSON line;
// this worker writes back exactly one JSON line per job on stdout. main.go
// only sends a worker its next job after reading the previous response, so a
// given worker is never asked to run two jobs at once - the Actual SDK only
// supports one open budget per process anyway.
//
// Pooling these processes only removes the cost of spawning node and loading
// @actual-app/api fresh for every request. It changes nothing about what's
// retained between jobs: fetchData() still does its own ephemeral,
// tmpfs-backed fetch-into-a-temp-dir-then-wipe-it per job (see actual.js), so
// no tenant data is held anywhere once a job's response has been written.

'use strict';

// Actual's SDK (or a dependency) may call console.log internally. stdout is
// reserved exclusively for our one-line-per-job JSON protocol, so redirect
// any stray console.log output to stderr instead of letting it corrupt the
// stream main.go is parsing.
console.log = (...args) => console.error(...args);

const fs = require('fs');
const readline = require('readline');
const actual = require('./actual.js');
const fetchData = actual.fetchData;

const rl = readline.createInterface({ input: process.stdin, terminal: false });

// Tracks whether we're in the middle of answering a job, so the crash
// handlers below know whether a stray error belongs to the job main.go is
// currently waiting on, or to something unrelated firing while idle.
let jobInFlight = false;

rl.on('line', async (line) => {
  jobInFlight = true;
  try {
    let params;
    try {
      params = JSON.parse(line);
    } catch (err) {
      process.stdout.write(JSON.stringify({ error: 'worker received invalid job payload: ' + (err.message || String(err)) }) + '\n');
      return;
    }
    try {
      const data = await fetchData(
        params.serverURL,
        params.serverPassword,
        params.budgetSyncId,
        params.budgetEncryptionPassword,
        params.groupName,
        params.included
      );
      process.stdout.write(JSON.stringify(data) + '\n');
    } catch (err) {
      // fetchData() already catches its own errors and resolves with
      // {error: ...}, so reaching here means something unexpected escaped it.
      process.stdout.write(JSON.stringify({ error: 'unexpected worker error: ' + (err.message || String(err)) }) + '\n');
    }
  } finally {
    jobInFlight = false;
  }
});

// Some errors the Actual SDK raises during a job (observed: a handler
// throwing on a bad/unknown budgetSyncId) escape fetchData()'s own
// try/catch entirely and surface here as a process-level unhandled
// rejection or uncaught exception instead. Left unhandled, Node's default
// is to crash the process, which main.go only sees as a bare stdout EOF -
// no information about what actually went wrong.
//
// If that happens while a job is in flight, report it as that job's answer
// before exiting, so the caller gets a real message instead of "EOF". If it
// happens while idle, don't write anything to stdout - there's no job
// waiting on a response, and writing a stray line here would desync the
// protocol by getting consumed as the answer to whatever job comes next.
// Either way we exit: something in the SDK's state may be inconsistent
// after a crash of this kind, so this process must not be reused. main.go
// detects the exit and never returns it to the worker pool (see main.go).
function crashAndExit(kind, err) {
  const message = err instanceof Error ? err.message : String(err);
  console.error(`${kind}, exiting worker:`, err);

  // A crash of this kind bypasses fetchData()'s own try/finally entirely
  // (that's exactly why we're here instead of in its catch block), so its
  // usual cleanup of the job's temp dir never runs either. Sweep it
  // synchronously - process.exit() below won't wait for an async cleanup -
  // so a crash can't leave real budget data sitting on disk.
  const tempDir = actual.getCurrentTempDir();
  if (tempDir) {
    try {
      fs.rmSync(tempDir, { recursive: true, force: true });
    } catch (rmErr) {
      console.error('Failed to clean up temp dir after crash:', rmErr);
    }
  }

  if (jobInFlight) {
    try {
      // __retireWorker tells main.go this worker must not be reused even
      // though it's answering with a valid line - see main.go's handling of
      // it. Whether the process has actually exited by the time main.go
      // checks is a race it can't win reliably, so this is a deterministic
      // signal in the message itself rather than something inferred from
      // process state/timing.
      process.stdout.write(JSON.stringify({ error: `worker crashed on ${kind}: ${message}`, __retireWorker: true }) + '\n');
    } catch (_) {
      // stdout itself may be broken; nothing more we can do.
    }
  }
  process.exit(1);
}

process.on('unhandledRejection', (reason) => crashAndExit('an unhandled rejection', reason));
process.on('uncaughtException', (err) => crashAndExit('an uncaught exception', err));

process.on('SIGTERM', () => process.exit(0));
