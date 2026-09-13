import { config as loadDotenv } from 'dotenv';
import { fileURLToPath } from 'node:url';

// Единый .env лежит в корне монорепозитория — тот же приём, что в backend/src/config.ts.
// Next.js сам грузит только frontend/.env*, поэтому здесь подтягиваем корневой файл руками,
// до того как Next начнёт инлайнить NEXT_PUBLIC_* при сборке.
loadDotenv({ path: fileURLToPath(new URL('../.env', import.meta.url)), quiet: true });

/** @type {import('next').NextConfig} */
const nextConfig = {};

export default nextConfig;
