import * as anchor from '@project-serum/anchor';
import { Program } from '@project-serum/anchor';
import { HelloWorld } from '../target/types/hello_world';

describe('hello-world', () => {

  // Configure the client to use the local cluster.
  anchor.setProvider(anchor.Provider.env());

  const program = anchor.workspace.HelloWorld as Program<HelloWorld>;

  it('Is initialized!', async () => {
    // Add your test here.
    const tx = await program.rpc.initialize({});
    console.log("Your transaction signature", tx);
  });
});
