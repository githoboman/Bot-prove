import os

def rename_file_contents_and_paths(root_dir):
    # Mapping for contents
    replacements = {
        "CasperProver": "BotProve",
        "casperprover": "botprove",
        "Casper testnet": "BOT Chain testnet",
        "Casper": "BOT Chain",
        "casper": "bot",
        "did:casper": "did:bot"
    }
    
    ignore_dirs = {'.git', 'node_modules', 'dist', 'build', '.next', 'artifacts', 'cache'}

    if not os.path.exists(root_dir):
        return

    for dirpath, dirnames, filenames in os.walk(root_dir, topdown=False):
        # Exclude ignored directories
        dirnames[:] = [d for d in dirnames if d not in ignore_dirs]
        
        # Rename files and update contents
        for filename in filenames:
            # skip some binaries or images if needed, but trying all files with utf-8 is fine (it will just fail and pass)
            old_path = os.path.join(dirpath, filename)
            
            # Read and replace contents
            try:
                with open(old_path, 'r', encoding='utf-8') as f:
                    content = f.read()
                
                new_content = content
                for old, new in replacements.items():
                    new_content = new_content.replace(old, new)
                
                if new_content != content:
                    with open(old_path, 'w', encoding='utf-8') as f:
                        f.write(new_content)
            except Exception as e:
                pass

            # Rename file
            new_filename = filename
            for old, new in replacements.items():
                new_filename = new_filename.replace(old, new)
            
            if new_filename != filename:
                new_path = os.path.join(dirpath, new_filename)
                try:
                    os.rename(old_path, new_path)
                except:
                    pass

        # Rename directories
        for dirname in dirnames:
            old_path = os.path.join(dirpath, dirname)
            new_dirname = dirname
            for old, new in replacements.items():
                new_dirname = new_dirname.replace(old, new)
            
            if new_dirname != dirname:
                new_path = os.path.join(dirpath, new_dirname)
                try:
                    os.rename(old_path, new_path)
                except:
                    pass

if __name__ == "__main__":
    rename_file_contents_and_paths("c:\\Users\\OWNER\\Desktop\\Bot prove")
    print("Done rebranding entire repo!")
