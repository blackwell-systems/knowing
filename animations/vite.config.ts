import {defineConfig} from 'vite';
import mc from '@motion-canvas/vite-plugin';

// Handle double-default export in some bundler configurations
const motionCanvas = (mc as any).default ?? mc;

export default defineConfig({
  plugins: [
    motionCanvas({
      project: ['./src/project.ts'],
    }),
  ],
});
