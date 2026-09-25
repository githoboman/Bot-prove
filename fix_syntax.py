import os

def fix_syntax(root_dir):
    replacements = {
        "github.com/make-software/bot-go-sdk": "github.com/make-software/casper-go-sdk",
        "@make-software/botclick": "@make-software/csprclick",
        "botclick-core": "csprclick-core",
        "botclick-ui": "csprclick-ui",
        "casper-go-sdk/v2/bot": "casper-go-sdk/v2/casper",
        "bot.NewSECP256k1": "casper.NewSECP256k1",
        "bot.NewED25519": "casper.NewED25519",
        "BOT ChainSubmitter": "BotChainSubmitter",
        "makeBOT ChainWithFake": "makeBotChainWithFake",
        "NewVmBOT ChainV1": "NewVmCasperV1",
        "BOT ChainVM": "BotChainVM",
        "BOT ChainV1": "CasperV1",
        "BOT ChainNetwork": "BotChainNetwork"
    }
    
    ignore_dirs = {'.git', 'node_modules', 'dist', 'build', '.next', 'artifacts', 'cache'}

    for dirpath, dirnames, filenames in os.walk(root_dir, topdown=True):
        dirnames[:] = [d for d in dirnames if d not in ignore_dirs]
        for filename in filenames:
            if not filename.endswith('.go') and not filename.endswith('.mod'):
                continue
            old_path = os.path.join(dirpath, filename)
            try:
                with open(old_path, 'r', encoding='utf-8') as f:
                    content = f.read()
                
                new_content = content
                for old, new in replacements.items():
                    new_content = new_content.replace(old, new)
                
                if new_content != content:
                    with open(old_path, 'w', encoding='utf-8') as f:
                        f.write(new_content)
            except:
                pass

if __name__ == "__main__":
    fix_syntax("c:\\Users\\OWNER\\Desktop\\Bot prove\\engine")
    print("Fixed engine syntax!")
