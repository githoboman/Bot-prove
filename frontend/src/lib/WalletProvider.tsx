import { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from 'react';

// Extend Window interface for standard EVM ethereum object
declare global {
  interface Window {
    ethereum?: any;
  }
}

interface WalletCtx {
  connected: boolean;
  publicKey: string | null;
  accountHash: string | null; // Kept for compatibility, can just mirror publicKey
  provider: string | null;
  clickRef: any | undefined; // Kept for compatibility
  ready: boolean;
  signIn: () => Promise<void>;
  signOut: () => void;
}

const WalletContext = createContext<WalletCtx>({
  connected: false,
  publicKey: null,
  accountHash: null,
  provider: null,
  clickRef: undefined,
  ready: false,
  signIn: async () => {},
  signOut: () => {},
});

export const useWallet = () => useContext(WalletContext);

// BOT Chain Network configuration
const BOT_CHAIN_TESTNET = {
  chainId: '0x3c8', // 968 in hex
  chainName: 'BOT Chain Testnet',
  nativeCurrency: { name: 'BOT', symbol: 'BOT', decimals: 18 },
  rpcUrls: ['https://rpc.bohr.life'],
  blockExplorerUrls: ['https://scan.botchain.ai'],
};

export default function CsprClickWrapper({ children }: { children: ReactNode }) {
  const [account, setAccount] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  // Check if wallet was already connected on load
  useEffect(() => {
    const checkConnection = async () => {
      if (window.ethereum) {
        try {
          const accounts = await window.ethereum.request({ method: 'eth_accounts' });
          if (accounts && accounts.length > 0) {
            setAccount(accounts[0]);
          }
        } catch (err) {
          console.error('Failed to check existing connection:', err);
        }
        
        // Listen for account changes
        window.ethereum.on('accountsChanged', (accounts: string[]) => {
          if (accounts.length > 0) {
            setAccount(accounts[0]);
          } else {
            setAccount(null);
          }
        });

        // Listen for network changes
        window.ethereum.on('chainChanged', () => {
          window.location.reload();
        });
      }
      setReady(true);
    };

    checkConnection();
  }, []);

  const signIn = useCallback(async () => {
    if (!window.ethereum) {
      alert('Please install MetaMask, Bitget Wallet, or TokenPocket to use the BOT Chain DApp.');
      return;
    }

    try {
      // Request account access
      const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
      const currentAccount = accounts[0];

      // Request network switch to BOT Chain Testnet
      try {
        await window.ethereum.request({
          method: 'wallet_switchEthereumChain',
          params: [{ chainId: BOT_CHAIN_TESTNET.chainId }],
        });
      } catch (switchError: any) {
        // This error code indicates that the chain has not been added to MetaMask.
        if (switchError.code === 4902) {
          try {
            await window.ethereum.request({
              method: 'wallet_addEthereumChain',
              params: [BOT_CHAIN_TESTNET],
            });
          } catch (addError) {
            console.error('Failed to add BOT Chain to wallet', addError);
          }
        }
      }

      setAccount(currentAccount);
    } catch (err) {
      console.error('Failed to connect wallet', err);
    }
  }, []);

  const signOut = useCallback(() => {
    // In EVM, we can't forcefully disconnect, but we can clear local state
    setAccount(null);
  }, []);

  const connected = !!account;

  return (
    <WalletContext.Provider
      value={{
        connected,
        publicKey: account,
        accountHash: account, // Mirroring publicKey for compatibility with BOT Chain UI components
        provider: 'EVM',
        clickRef: undefined,
        ready,
        signIn,
        signOut,
      }}
    >
      {children}
    </WalletContext.Provider>
  );
}
