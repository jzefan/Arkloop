import { loadConfig } from './config.js';
import { createHttpServer } from './server.js';

process.on('uncaughtException', (err) => {
  process.stderr.write(JSON.stringify({ level: 'error', event: 'uncaught_exception', error: err.message }) + '\n');
  process.exit(1);
});

process.on('unhandledRejection', (reason) => {
  const message = reason instanceof Error ? reason.message : String(reason);
  process.stderr.write(JSON.stringify({ level: 'error', event: 'unhandled_rejection', error: message }) + '\n');
  process.exit(1);
});

const config = loadConfig();
const server = createHttpServer();

server.listen(config.port, '0.0.0.0', () => {
  process.stdout.write(JSON.stringify({ level: 'info', event: 'server_started', port: config.port }) + '\n');
});

process.on('SIGTERM', () => {
  server.close(() => {
    process.stdout.write(JSON.stringify({ level: 'info', event: 'server_stopped' }) + '\n');
    process.exit(0);
  });
});

process.on('SIGINT', () => {
  server.close(() => {
    process.exit(0);
  });
});
