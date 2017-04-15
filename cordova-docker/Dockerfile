FROM oddlid/arch-cli-minimal:latest

# Temporarily uninstall ca-certificates-utils to get rid of the 
# ca-certificates-utils: /etc/ssl/certs/ca-certificates.crt exists in filesystem error
# See: https://bbs.archlinux.org/viewtopic.php?id=223895
# It will be reinstalled later on
RUN  pacman -Rdd --noconfirm ca-certificates-utils

# Install packages for both ease of use and 
# structurally needed for PhantomJS (fontconfig) and Node
RUN pacman -Syy
RUN pacman -S --noconfirm --needed ca-certificates-utils harfbuzz fontconfig nodejs npm
RUN pacman -S --noconfirm --needed nano htop wget

# Archlinux CN repo (has yaourt and sometimes other interesting tools)
RUN echo "[archlinuxcn]" >> /etc/pacman.conf && \
echo "SigLevel = Optional TrustAll" >> /etc/pacman.conf && \
echo "Server = http://repo.archlinuxcn.org/\$arch" >> /etc/pacman.conf

# Add BBQLinux repo for Android development: http://bbqlinux.org/
RUN echo "[bbqlinux]" >> /etc/pacman.conf && \
echo "Server = http://packages.bbqlinux.org/\$repo/os/\$arch" >> /etc/pacman.conf && \
pacman-key -r 04C0A941 && \
pacman-key --lsign-key 04C0A941

# Add multilib repo for the Android SDK (it requires 32bit libs)
RUN sed -i '/#\[multilib\]/,/#Include = \/etc\/pacman.d\/mirrorlist/ s/#//' /etc/pacman.conf && \
sed -i '/#\[multilib\]/,/#Include = \/etc\/pacman.d\/mirrorlist/ s/#//' /etc/pacman.conf && \
sed -i 's/#\[multilib\]/\[multilib\]/g' /etc/pacman.conf

# Update and force a refresh of all package lists even if they appear up to date.
RUN pacman -Syyu --noconfirm

# Install all the repo keyrings and mirrorlists
RUN pacman --noconfirm -S archlinuxcn-keyring bbqlinux-keyring

# Install yaourt, package-query and cower for easy AUR usage.
# TODO make sure package query still exists later after yaourt uninstall
RUN pacman -S --noconfirm yaourt package-query cower

# Ensure the system locale is English
RUN locale-gen en_US.UTF-8
ENV LANG='en_US.UTF-8' LANGUAGE='en_US:en' LC_ALL='en_US.UTF-8'

# Set up basic aliases for convenience
RUN echo "alias ll='ls -alF'" >> ~/.bashrc
RUN echo "alias la='ls -A'" >> ~/.bashrc
RUN echo "alias l='ls -CF'" >> ~/.bashrc

# Install pacman base devel to allow building packages from AUR
# RUN pacman -S --noconfirm --needed base-devel git

# Install Oracle Java
RUN yaourt -S --noconfirm --needed jdk

# Install Android SDK
RUN pacman -S --noconfirm android-sdk

# Install Android build tools
RUN yes | /opt/android-sdk/tools/bin/sdkmanager "platforms;android-25" "build-tools;25.0.2" "extras;google;m2repository" "extras;android;m2repository"

# Set up Cordova
RUN npm install -g cordova
