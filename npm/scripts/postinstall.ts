import { ensureBinary } from '../src/download';

ensureBinary()
  .then(() => {
    console.log('✓ Pinchtab setup complete');
    process.exit(0);
  })
  .catch((err) => {
    console.error('✗ Failed to setup Pinchtab:', err.message);
    process.exit(1);
  });
