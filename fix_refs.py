import os

def fix_references(root_dir):
    replacements = {
        "github.com/githoboman/Bot-prove": "github.com/githoboman/Bot-prove",
        "github.com/githoboman/Bot-prove": "github.com/githoboman/Bot-prove",
        "scan.botchain.ai": "scan.botchain.ai",
        "rpc.bohr.life": "rpc.bohr.life",
        "bot-prove.vercel.app": "bot-prove.vercel.app"
    }
    
    ignore_dirs = {'.git', 'node_modules', 'dist', 'build', '.next', 'artifacts', 'cache'}

    for dirpath, dirnames, filenames in os.walk(root_dir, topdown=False):
        dirnames[:] = [d for d in dirnames if d not in ignore_dirs]
        for filename in filenames:
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
    fix_references("c:\\Users\\OWNER\\Desktop\\Bot prove")
    print("Fixed github and explorer links!")
